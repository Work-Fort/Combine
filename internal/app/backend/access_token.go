package backend

import (
	"context"
	"errors"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/utils"
)

// CreateAccessToken creates an access token for user.
func (b *Backend) CreateAccessToken(ctx context.Context, user *domain.User, name string, expiresAt time.Time) (string, error) {
	token := GenerateToken()
	tokenHash := HashToken(token)
	name = utils.Sanitize(name)

	_, err := b.store.CreateAccessToken(ctx, name, user.ID, tokenHash, expiresAt)
	if err != nil {
		return "", err
	}

	return token, nil
}

// DeleteAccessToken deletes an access token for a user.
func (b *Backend) DeleteAccessToken(ctx context.Context, user *domain.User, id int64) error {
	_, err := b.store.GetAccessToken(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrTokenNotFound
		}
		return err
	}

	if err := b.store.DeleteAccessTokenForUser(ctx, user.ID, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrTokenNotFound
		}
		return err
	}

	return nil
}

// ListAccessTokens lists access tokens for a user.
func (b *Backend) ListAccessTokens(ctx context.Context, user *domain.User) ([]*domain.AccessToken, error) {
	return b.store.ListAccessTokensByUserID(ctx, user.ID)
}
