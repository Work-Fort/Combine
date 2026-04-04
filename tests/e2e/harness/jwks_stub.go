package harness

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// StartJWKSStub starts an HTTP server that serves a JWKS endpoint and a
// verify-api-key endpoint. It returns the listener address, a stop function,
// and a signJWT function that creates signed JWTs with the stub's private key.
func StartJWKSStub() (addr string, stop func(), signJWT func(id, username, displayName, userType string) string) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("generate RSA key: " + err.Error())
	}

	rsaJWK, err := jwk.FromRaw(privKey)
	if err != nil {
		panic("jwk from raw: " + err.Error())
	}
	_ = rsaJWK.Set(jwk.KeyIDKey, "test-key-1")
	_ = rsaJWK.Set(jwk.AlgorithmKey, jwa.RS256)

	pubJWK, err := jwk.PublicKeyOf(rsaJWK)
	if err != nil {
		panic("public key of: " + err.Error())
	}
	_ = pubJWK.Set(jwk.KeyIDKey, "test-key-1")
	_ = pubJWK.Set(jwk.AlgorithmKey, jwa.RS256)

	pubSet := jwk.NewSet()
	_ = pubSet.AddKey(pubJWK)

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pubSet)
	})

	mux.HandleFunc("/v1/verify-api-key", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key string `json:"key"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"valid": true,
			"key": map[string]any{
				"userId": "stub-api-key-user",
				"metadata": map[string]any{
					"username":     "stub-user",
					"name":         "Stub User",
					"display_name": "Stub User",
					"type":         "user",
				},
			},
		})
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("listen: " + err.Error())
	}

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	addr = ln.Addr().String()

	stop = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}

	signJWT = func(id, username, displayName, userType string) string {
		now := time.Now()
		tok, err := jwt.NewBuilder().
			Subject(id).
			Issuer("combine-test-stub").
			Audience([]string{"combine"}).
			IssuedAt(now).
			Expiration(now.Add(1 * time.Hour)).
			Claim("username", username).
			Claim("name", displayName).
			Claim("display_name", displayName).
			Claim("type", userType).
			Build()
		if err != nil {
			panic("build JWT: " + err.Error())
		}

		signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, rsaJWK))
		if err != nil {
			panic("sign JWT: " + err.Error())
		}
		return string(signed)
	}

	return addr, stop, signJWT
}
