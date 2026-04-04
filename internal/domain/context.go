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

var identityContextKey = &contextKey{"identity"}

// IdentityFromContext returns the identity from the context.
func IdentityFromContext(ctx context.Context) *Identity {
	if id, ok := ctx.Value(identityContextKey).(*Identity); ok {
		return id
	}
	return nil
}

// WithIdentityContext returns a new context with the given identity.
func WithIdentityContext(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityContextKey, id)
}

// UserContextKey returns the context key used for users.
// This is exposed for SSH session context which uses SetValue directly.
func UserContextKey() *contextKey {
	return userContextKey
}

// StoreContextKey returns the context key used for the store.
// This is exposed for SSH session context which uses SetValue directly.
func StoreContextKey() *contextKey {
	return storeContextKey
}
