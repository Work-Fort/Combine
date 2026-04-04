package domain

import "context"

type contextKey struct{ name string }

var (
	repoContextKey        = &contextKey{"repo"}
	userContextKey        = &contextKey{"user"}
	storeContextKey       = &contextKey{"store"}
	accessLevelContextKey = &contextKey{"access-level"}
)

// RepoFromContext returns the repository from the context.
func RepoFromContext(ctx context.Context) *Repo {
	if r, ok := ctx.Value(repoContextKey).(*Repo); ok {
		return r
	}
	return nil
}

// WithRepoContext returns a new context with the given repository.
func WithRepoContext(ctx context.Context, r *Repo) context.Context {
	return context.WithValue(ctx, repoContextKey, r)
}

// UserFromContext returns the user from the context.
func UserFromContext(ctx context.Context) *User {
	if u, ok := ctx.Value(userContextKey).(*User); ok {
		return u
	}
	return nil
}

// WithUserContext returns a new context with the given user.
func WithUserContext(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// StoreFromContext returns the store from the context.
func StoreFromContext(ctx context.Context) Store {
	if s, ok := ctx.Value(storeContextKey).(Store); ok {
		return s
	}
	return nil
}

// WithStoreContext returns a new context with the given store.
func WithStoreContext(ctx context.Context, s Store) context.Context {
	return context.WithValue(ctx, storeContextKey, s)
}

// AccessLevelFromContext returns the access level from the context.
func AccessLevelFromContext(ctx context.Context) AccessLevel {
	if ac, ok := ctx.Value(accessLevelContextKey).(AccessLevel); ok {
		return ac
	}
	return AccessLevel(-1)
}

// WithAccessLevelContext returns a new context with the given access level.
func WithAccessLevelContext(ctx context.Context, ac AccessLevel) context.Context {
	return context.WithValue(ctx, accessLevelContextKey, ac)
}
