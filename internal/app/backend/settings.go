package backend

import (
	"context"

	"github.com/Work-Fort/Combine/internal/domain"
)

// AllowKeyless returns whether or not keyless access is allowed.
func (b *Backend) AllowKeyless(ctx context.Context) bool {
	allow, err := b.store.GetAllowKeylessAccess(ctx)
	if err != nil {
		return false
	}
	return allow
}

// SetAllowKeyless sets whether or not keyless access is allowed.
func (b *Backend) SetAllowKeyless(ctx context.Context, allow bool) error {
	return b.store.SetAllowKeylessAccess(ctx, allow)
}

// AnonAccess returns the level of anonymous access.
func (b *Backend) AnonAccess(ctx context.Context) domain.AccessLevel {
	level, err := b.store.GetAnonAccess(ctx)
	if err != nil {
		return domain.NoAccess
	}
	return level
}

// SetAnonAccess sets the level of anonymous access.
func (b *Backend) SetAnonAccess(ctx context.Context, level domain.AccessLevel) error {
	return b.store.SetAnonAccess(ctx, level)
}
