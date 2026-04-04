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

// DeleteKey deletes an SSH key by ID.
func (c *APIClient) DeleteKey(t *testing.T, keyID string) {
	t.Helper()
	resp := c.DoRequest(t, "DELETE", fmt.Sprintf("/api/v1/user/keys/%s", keyID), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DeleteKey %s: status %d, want 204", keyID, resp.StatusCode)
	}
}
