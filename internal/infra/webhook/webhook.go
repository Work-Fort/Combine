package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/git"
	"github.com/Work-Fort/Combine/internal/infra/utils"
	"github.com/Work-Fort/Combine/internal/infra/version"
	"github.com/Work-Fort/Combine/internal/legacy/config"
	"github.com/google/go-querystring/query"
	"github.com/google/uuid"
)

// Hook is a repository webhook.
type Hook struct {
	domain.Webhook
	ContentType ContentType
	Events      []Event
}

// Delivery is a webhook delivery.
type Delivery struct {
	domain.WebhookDelivery
	Event Event
}

// secureHTTPClient creates an HTTP client with SSRF protection.
var secureHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Parse the address to get the IP
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err //nolint:wrapcheck
			}

			// Validate the resolved IP before connecting
			ip := net.ParseIP(host)
			if ip != nil {
				if err := ValidateIPBeforeDial(ip); err != nil {
					return nil, fmt.Errorf("blocked connection to private IP: %w", err)
				}
			}

			// Use standard dialer with timeout
			dialer := &net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
	// Don't follow redirects to prevent bypassing IP validation
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// do sends a webhook.
// Caller must close the returned body.
func do(ctx context.Context, url string, method string, headers http.Header, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header = headers
	res, err := secureHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// SendWebhook sends a webhook event.
func SendWebhook(ctx context.Context, w domain.Webhook, event Event, payload interface{}) error {
	var buf bytes.Buffer
	datastore := domain.StoreFromContext(ctx)

	contentType := ContentType(w.ContentType) //nolint:gosec
	switch contentType {
	case ContentTypeJSON:
		if err := json.NewEncoder(&buf).Encode(payload); err != nil {
			return err
		}
	case ContentTypeForm:
		v, err := query.Values(payload)
		if err != nil {
			return err
		}
		buf.WriteString(v.Encode()) //nolint: errcheck
	default:
		return ErrInvalidContentType
	}

	headers := http.Header{}
	headers.Add("Content-Type", contentType.String())
	headers.Add("User-Agent", "SoftServe/"+version.Version)
	headers.Add("X-SoftServe-Event", event.String())

	id, err := uuid.NewUUID()
	if err != nil {
		return err
	}

	headers.Add("X-SoftServe-Delivery", id.String())

	reqBody := buf.String()
	if w.Secret != "" {
		sig := hmac.New(sha256.New, []byte(w.Secret))
		sig.Write([]byte(reqBody)) //nolint: errcheck
		headers.Add("X-SoftServe-Signature", "sha256="+hex.EncodeToString(sig.Sum(nil)))
	}

	res, reqErr := do(ctx, w.URL, http.MethodPost, headers, &buf)
	var reqHeaders string
	for k, v := range headers {
		reqHeaders += k + ": " + v[0] + "\n"
	}

	resStatus := 0
	resHeaders := ""
	resBody := ""

	if res != nil {
		resStatus = res.StatusCode
		for k, v := range res.Header {
			resHeaders += k + ": " + v[0] + "\n"
		}

		if res.Body != nil {
			defer res.Body.Close() //nolint: errcheck
			b, err := io.ReadAll(res.Body)
			if err != nil {
				return err
			}

			resBody = string(b)
		}
	}

	return datastore.CreateWebhookDelivery(ctx, id, w.ID, int(event), w.URL, http.MethodPost, reqErr, reqHeaders, reqBody, resStatus, resHeaders, resBody)
}

// SendEvent sends a webhook event.
func SendEvent(ctx context.Context, payload EventPayload) error {
	datastore := domain.StoreFromContext(ctx)
	webhooks, err := datastore.ListWebhooksByRepoIDWhereEvent(ctx, payload.RepositoryID(), []int{int(payload.Event())})
	if err != nil {
		return err
	}

	for _, w := range webhooks {
		if err := SendWebhook(ctx, *w, payload.Event(), payload); err != nil {
			return err
		}
	}

	return nil
}

func repoURL(publicURL string, repo string) string {
	return fmt.Sprintf("%s/%s.git", publicURL, utils.SanitizeRepo(repo))
}

// openRepoFromContext opens a git repository using config from context.
func openRepoFromContext(ctx context.Context, name string) (*git.Repository, error) {
	cfg := config.FromContext(ctx)
	rn := strings.ReplaceAll(name, "/", string(filepath.Separator))
	rp := filepath.Join(cfg.DataPath, "repos", rn+".git")
	return git.Open(rp)
}

func getDefaultBranch(ctx context.Context, repo *domain.Repo) (string, error) {
	r, err := openRepoFromContext(ctx, repo.Name)
	if err != nil {
		return "", err
	}
	ref, err := r.HEAD()
	// XXX: we check for ErrReferenceNotExist here because we don't want to
	// return an error if the repo is an empty repo.
	if err != nil && !errors.Is(err, git.ErrReferenceNotExist) {
		return "", err
	}

	if ref == nil {
		return "", nil
	}

	return ref.Name().Short(), nil
}
