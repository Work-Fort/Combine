package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// APIClient makes authenticated REST API calls against a Combine server.
type APIClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewAPIClient creates an API client for the given HTTP address and JWT token.
func NewAPIClient(httpAddr, token string) *APIClient {
	return &APIClient{
		baseURL: "http://" + httpAddr,
		token:   token,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// APIClient returns an authenticated APIClient for the given username.
func (d *Daemon) APIClient(t *testing.T, username string) *APIClient {
	t.Helper()
	token := d.SignJWT("uuid-"+username, username, username, "user")
	return NewAPIClient(d.HTTPAddr, token)
}

// DoRequest performs an HTTP request and returns the response.
func (c *APIClient) DoRequest(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func (c *APIClient) decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode JSON response: %v\nbody: %s", err, body)
	}
	return result
}

func (c *APIClient) decodeJSONArray(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var result []map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode JSON array response: %v\nbody: %s", err, body)
	}
	return result
}

// CreateRepo creates a repository via the REST API.
func (c *APIClient) CreateRepo(t *testing.T, name string, private bool) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "POST", "/api/v1/repos", map[string]any{
		"name":    name,
		"private": private,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("CreateRepo %s: status %d, body: %s", name, resp.StatusCode, body)
	}
	return c.decodeJSON(t, resp)
}

// GetRepo retrieves a repository by name.
func (c *APIClient) GetRepo(t *testing.T, name string) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", "/api/v1/repos/"+name, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GetRepo %s: status %d, body: %s", name, resp.StatusCode, body)
	}
	return c.decodeJSON(t, resp)
}

// ListRepos lists all repositories.
func (c *APIClient) ListRepos(t *testing.T) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", "/api/v1/repos", nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ListRepos: status %d, body: %s", resp.StatusCode, body)
	}
	return c.decodeJSONArray(t, resp)
}

// UpdateRepo updates a repository.
func (c *APIClient) UpdateRepo(t *testing.T, name string, updates map[string]any) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "PATCH", "/api/v1/repos/"+name, updates)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("UpdateRepo %s: status %d, body: %s", name, resp.StatusCode, body)
	}
	return c.decodeJSON(t, resp)
}

// DeleteRepo deletes a repository.
func (c *APIClient) DeleteRepo(t *testing.T, name string) {
	t.Helper()
	resp := c.DoRequest(t, "DELETE", "/api/v1/repos/"+name, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DeleteRepo %s: status %d, want 204", name, resp.StatusCode)
	}
}

// ListKeys lists the authenticated user's SSH keys.
func (c *APIClient) ListKeys(t *testing.T) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", "/api/v1/user/keys", nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ListKeys: status %d, body: %s", resp.StatusCode, body)
	}
	return c.decodeJSONArray(t, resp)
}

// AddKey adds an SSH public key for the authenticated user.
func (c *APIClient) AddKey(t *testing.T, publicKey string) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "POST", "/api/v1/user/keys", map[string]any{
		"key": publicKey,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("AddKey: status %d, body: %s", resp.StatusCode, body)
	}
	return c.decodeJSON(t, resp)
}

// CreateIssue creates an issue via the REST API.
func (c *APIClient) CreateIssue(t *testing.T, repo, title, body string) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "POST", fmt.Sprintf("/api/v1/repos/%s/issues", repo), map[string]any{
		"title": title,
		"body":  body,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("CreateIssue %s: status %d, body: %s", repo, resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// GetIssue retrieves an issue by repo and number.
func (c *APIClient) GetIssue(t *testing.T, repo string, number int) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/issues/%d", repo, number), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GetIssue %s#%d: status %d, body: %s", repo, number, resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// ListIssues lists issues for a repo.
func (c *APIClient) ListIssues(t *testing.T, repo string) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/issues", repo), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ListIssues %s: status %d, body: %s", repo, resp.StatusCode, b)
	}
	return c.decodeJSONArray(t, resp)
}

// ListIssuesWithStatus lists issues for a repo filtered by status.
func (c *APIClient) ListIssuesWithStatus(t *testing.T, repo, status string) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/issues?status=%s", repo, status), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ListIssuesWithStatus %s status=%s: status %d, body: %s", repo, status, resp.StatusCode, b)
	}
	return c.decodeJSONArray(t, resp)
}

// UpdateIssue updates an issue.
func (c *APIClient) UpdateIssue(t *testing.T, repo string, number int, updates map[string]any) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "PATCH", fmt.Sprintf("/api/v1/repos/%s/issues/%d", repo, number), updates)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("UpdateIssue %s#%d: status %d, body: %s", repo, number, resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// CreateComment creates a comment on an issue.
func (c *APIClient) CreateComment(t *testing.T, repo string, number int, body string) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "POST", fmt.Sprintf("/api/v1/repos/%s/issues/%d/comments", repo, number), map[string]any{
		"body": body,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("CreateComment %s#%d: status %d, body: %s", repo, number, resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// ListComments lists comments on an issue.
func (c *APIClient) ListComments(t *testing.T, repo string, number int) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/issues/%d/comments", repo, number), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ListComments %s#%d: status %d, body: %s", repo, number, resp.StatusCode, b)
	}
	return c.decodeJSONArray(t, resp)
}

// CreatePullRequest creates a pull request via the REST API.
func (c *APIClient) CreatePullRequest(t *testing.T, repo, title, body, source, target string) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "POST", fmt.Sprintf("/api/v1/repos/%s/pulls", repo), map[string]any{
		"title":         title,
		"body":          body,
		"source_branch": source,
		"target_branch": target,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("CreatePullRequest: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// GetPullRequest retrieves a pull request by repo and number.
func (c *APIClient) GetPullRequest(t *testing.T, repo string, number int) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d", repo, number), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GetPullRequest: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// ListPullRequests lists pull requests for a repo.
func (c *APIClient) ListPullRequests(t *testing.T, repo string) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/pulls", repo), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ListPullRequests: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSONArray(t, resp)
}

// MergePullRequest merges a pull request.
func (c *APIClient) MergePullRequest(t *testing.T, repo string, number int, method string) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "POST", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/merge", repo, number), map[string]any{
		"merge_method": method,
	})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("MergePullRequest: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// SubmitReview submits a review on a pull request.
func (c *APIClient) SubmitReview(t *testing.T, repo string, number int, state, body string, comments []map[string]any) map[string]any {
	t.Helper()
	payload := map[string]any{
		"state": state,
		"body":  body,
	}
	if comments != nil {
		payload["comments"] = comments
	}
	resp := c.DoRequest(t, "POST", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/reviews", repo, number), payload)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("SubmitReview: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// GetPullRequestDiff retrieves the diff for a pull request.
func (c *APIClient) GetPullRequestDiff(t *testing.T, repo string, number int) string {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/diff", repo, number), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GetPullRequestDiff: status %d, body: %s", resp.StatusCode, b)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b)
}

// GetPullRequestFiles retrieves the changed files for a pull request.
func (c *APIClient) GetPullRequestFiles(t *testing.T, repo string, number int) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/files", repo, number), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GetPullRequestFiles: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSONArray(t, resp)
}

// CreateWebhook creates a webhook via the REST API.
func (c *APIClient) CreateWebhook(t *testing.T, repo string, url string, events []string) map[string]any {
	t.Helper()
	body := map[string]any{
		"url":    url,
		"events": events,
		"active": true,
	}
	resp := c.DoRequest(t, "POST", "/api/v1/repos/"+repo+"/webhooks", body)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("CreateWebhook: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// ListWebhooks lists webhooks for a repo.
func (c *APIClient) ListWebhooks(t *testing.T, repo string) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", "/api/v1/repos/"+repo+"/webhooks", nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ListWebhooks: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSONArray(t, resp)
}

// GetWebhook retrieves a webhook by ID.
func (c *APIClient) GetWebhook(t *testing.T, repo string, id int64) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/webhooks/%d", repo, id), nil)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GetWebhook: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// UpdateWebhook updates a webhook.
func (c *APIClient) UpdateWebhook(t *testing.T, repo string, id int64, body map[string]any) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "PATCH", fmt.Sprintf("/api/v1/repos/%s/webhooks/%d", repo, id), body)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("UpdateWebhook: status %d, body: %s", resp.StatusCode, b)
	}
	return c.decodeJSON(t, resp)
}

// DeleteWebhook deletes a webhook.
func (c *APIClient) DeleteWebhook(t *testing.T, repo string, id int64) {
	t.Helper()
	resp := c.DoRequest(t, "DELETE", fmt.Sprintf("/api/v1/repos/%s/webhooks/%d", repo, id), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DeleteWebhook: status %d, want 204", resp.StatusCode)
	}
}

// DeleteKey deletes an SSH key by ID.
func (c *APIClient) DeleteKey(t *testing.T, keyID string) {
	t.Helper()
	resp := c.DoRequest(t, "DELETE", fmt.Sprintf("/api/v1/user/keys/%s", keyID), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DeleteKey %s: status %d, want 204", keyID, resp.StatusCode)
	}
}
