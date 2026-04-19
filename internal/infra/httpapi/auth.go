package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"charm.land/log/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/config"
	"github.com/Work-Fort/Combine/internal/domain"
)

// authenticate authenticates the identity from the request.
func authenticate(r *http.Request) (*domain.Identity, error) {
	identity, err := parseAuthHdr(r)
	if err != nil || identity == nil {
		if errors.Is(err, ErrInvalidToken) {
			return nil, err
		}
		return nil, domain.ErrUserNotFound
	}

	return identity, nil
}

// ErrInvalidHeader is returned when the authorization header is invalid.
var ErrInvalidHeader = errors.New("invalid authorization header")

func parseAuthHdr(r *http.Request) (*domain.Identity, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return nil, ErrInvalidHeader
	}

	ctx := r.Context()
	logger := log.FromContext(ctx).WithPrefix("http.auth")
	be := backend.FromContext(ctx)

	logger.Debug("authorization auth header", "header", header)

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid authorization header")
	}

	switch strings.ToLower(parts[0]) {
	case "bearer":
		claims, err := parseJWT(ctx, parts[1])
		if err != nil {
			return nil, err
		}

		// Subject format: "username#identityID"
		subParts := strings.SplitN(claims.Subject, "#", 2)
		if len(subParts) != 2 {
			logger.Error("invalid jwt subject", "subject", claims.Subject)
			return nil, errors.New("invalid jwt subject")
		}

		identity, err := be.IdentityByUsername(ctx, subParts[0])
		if err != nil {
			logger.Error("failed to get identity", "err", err)
			return nil, err
		}

		if identity.ID != subParts[1] {
			logger.Error("invalid jwt subject: identity ID mismatch", "subject", claims.Subject)
			return nil, errors.New("invalid jwt subject")
		}

		return identity, nil
	default:
		return nil, ErrInvalidHeader
	}
}

// ErrInvalidToken is returned when a token is invalid.
var ErrInvalidToken = errors.New("invalid token")

func parseJWT(ctx context.Context, bearer string) (*jwt.RegisteredClaims, error) {
	cfg := config.FromContext(ctx)
	logger := log.FromContext(ctx).WithPrefix("http.auth")
	kp, err := config.KeyPair(cfg)
	if err != nil {
		return nil, err
	}

	repo := domain.RepoFromContext(ctx)
	if repo == nil {
		return nil, errors.New("missing repository")
	}

	token, err := jwt.ParseWithClaims(bearer, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, errors.New("invalid signing method")
		}

		return kp.CryptoPublicKey(), nil
	},
		jwt.WithIssuer(cfg.HTTP.PublicURL),
		jwt.WithIssuedAt(),
		jwt.WithAudience(repo.Name),
	)
	if err != nil {
		logger.Error("failed to parse jwt", "err", err)
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !token.Valid || !ok {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
