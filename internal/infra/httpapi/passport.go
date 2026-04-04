package web

import (
	"context"
	"fmt"
	"net/http"

	auth "github.com/Work-Fort/Passport/go/service-auth"
	"github.com/Work-Fort/Passport/go/service-auth/apikey"
	"github.com/Work-Fort/Passport/go/service-auth/jwt"

	"github.com/Work-Fort/Combine/internal/domain"
)

// PassportAuth wraps Passport's auth middleware and auto-provisions
// identities in Combine's store on each authenticated request.
type PassportAuth struct {
	mw    auth.Middleware
	store domain.Store
	jwtV  *jwt.Validator
}

// NewPassportAuth creates a PassportAuth that validates tokens against the
// Passport service at passportURL and upserts identities into store.
func NewPassportAuth(ctx context.Context, passportURL string, store domain.Store) (*PassportAuth, error) {
	opts := auth.DefaultOptions(passportURL)
	jwtV, err := jwt.New(ctx, opts.JWKSURL, opts.JWKSRefreshInterval)
	if err != nil {
		return nil, fmt.Errorf("init JWT validator: %w", err)
	}
	akV := apikey.New(opts.VerifyAPIKeyURL, opts.APIKeyCacheTTL)
	mw := auth.NewFromValidators(jwtV, akV)

	return &PassportAuth{mw: mw, store: store, jwtV: jwtV}, nil
}

// Close releases resources held by the Passport auth validators.
func (p *PassportAuth) Close() {
	if p != nil && p.jwtV != nil {
		p.jwtV.Close()
	}
}

// Middleware returns an http.Handler middleware that validates Passport tokens
// and auto-provisions the identity in Combine's store.
func (p *PassportAuth) Middleware(next http.Handler) http.Handler {
	return p.mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passportID, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		// Auto-provision identity
		identity, err := p.store.UpsertIdentity(r.Context(),
			passportID.ID, passportID.Username, passportID.DisplayName, passportID.Type)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		ctx := domain.WithIdentityContext(r.Context(), identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	}))
}
