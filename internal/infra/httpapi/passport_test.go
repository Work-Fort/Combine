package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/Work-Fort/Combine/internal/domain"
)

// passportStub stands in for the real Passport service. It serves
// /.well-known/jwks.json with a fixed minimal JWKS and
// /v1/verify-api-key returning 200 with a service identity for one
// canned key, 401 otherwise. It also records every verify-api-key call
// so the test can assert no fallthrough.
type passportStub struct {
	*httptest.Server
	verifyCount int64
	validKey    string
}

func newPassportStub(t *testing.T) *passportStub {
	t.Helper()
	s := &passportStub{validKey: "wf-svc_test"}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jwks", func(w http.ResponseWriter, r *http.Request) {
		// Empty JWKS is enough — these tests don't exercise JWT acceptance.
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
	})
	mux.HandleFunc("/v1/verify-api-key", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&s.verifyCount, 1)
		var body struct {
			Key string `json:"key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Key != s.validKey {
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"valid": true,
			"key": map[string]any{
				"userId": "svc-1",
				"metadata": map[string]any{
					"username": "test-service",
					"type":     "service",
				},
			},
		})
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *passportStub) VerifyCount() int64 { return atomic.LoadInt64(&s.verifyCount) }

// inMemoryStore satisfies domain.IdentityStore (the narrow interface that
// NewPassportAuth now accepts — see Step 2). Only UpsertIdentity is
// exercised by the middleware; the remaining methods panic to surface any
// unexpected calls during testing.
type inMemoryStore struct{}

func (inMemoryStore) UpsertIdentity(_ context.Context, id, username, displayName, typ string) (*domain.Identity, error) {
	return &domain.Identity{ID: id, Username: username, DisplayName: displayName, Type: typ}, nil
}

func (inMemoryStore) GetIdentityByID(_ context.Context, _ string) (*domain.Identity, error) {
	panic("not implemented in test stub")
}

func (inMemoryStore) GetIdentityByUsername(_ context.Context, _ string) (*domain.Identity, error) {
	panic("not implemented in test stub")
}

func (inMemoryStore) GetIdentityByPublicKey(_ context.Context, _ ssh.PublicKey) (*domain.Identity, error) {
	panic("not implemented in test stub")
}

func (inMemoryStore) ListIdentities(_ context.Context) ([]*domain.Identity, error) {
	panic("not implemented in test stub")
}

func (inMemoryStore) SetIdentityAdmin(_ context.Context, _ string, _ bool) error {
	panic("not implemented in test stub")
}

func (inMemoryStore) AddIdentityPublicKey(_ context.Context, _ string, _ ssh.PublicKey) error {
	panic("not implemented in test stub")
}

func (inMemoryStore) RemoveIdentityPublicKey(_ context.Context, _ string, _ int64) error {
	panic("not implemented in test stub")
}

func (inMemoryStore) ListIdentityPublicKeys(_ context.Context, _ string) ([]*domain.PublicKey, error) {
	panic("not implemented in test stub")
}

func TestPassportAuth_BearerForAPIKeyReturns401(t *testing.T) {
	stub := newPassportStub(t)
	pa, err := NewPassportAuth(context.Background(), stub.URL, inMemoryStore{})
	if err != nil {
		t.Fatalf("NewPassportAuth: %v", err)
	}
	defer pa.Close()

	handler := pa.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called when API key is sent under Bearer")
	}))

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/v1/x", nil)
	req.Header.Set("Authorization", "Bearer "+stub.validKey) // wrong scheme
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := stub.VerifyCount(); got != 0 {
		t.Errorf("verify-api-key called %d times; want 0 (no fallthrough)", got)
	}
}

func TestPassportAuth_ApiKeyV1Routes(t *testing.T) {
	stub := newPassportStub(t)
	pa, err := NewPassportAuth(context.Background(), stub.URL, inMemoryStore{})
	if err != nil {
		t.Fatalf("NewPassportAuth: %v", err)
	}
	defer pa.Close()

	called := false
	handler := pa.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/v1/x", nil)
	req.Header.Set("Authorization", "ApiKey-v1 "+stub.validKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !called {
		t.Error("downstream handler was not called")
	}
	if got := stub.VerifyCount(); got != 1 {
		t.Errorf("verify-api-key called %d times; want 1", got)
	}
	if !strings.HasPrefix(stub.URL, "http") {
		t.Errorf("stub URL malformed: %s", stub.URL) // sanity
	}
	_ = fmt.Sprintf // keep fmt import live if test scaffolding evolves
}
