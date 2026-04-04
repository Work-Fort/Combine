package backend

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/sshutils"
	"github.com/Work-Fort/Combine/internal/infra/utils"
	"golang.org/x/crypto/ssh"
)

// AccessLevel returns the access level of a user for a repository.
func (d *Backend) AccessLevel(ctx context.Context, repo string, username string) domain.AccessLevel {
	user, _ := d.User(ctx, username)
	return d.AccessLevelForUser(ctx, repo, user)
}

// AccessLevelByPublicKey returns the access level of a user's public key for a repository.
func (d *Backend) AccessLevelByPublicKey(ctx context.Context, repo string, pk ssh.PublicKey) domain.AccessLevel {
	for _, k := range d.cfg.AdminKeys {
		if sshutils.KeysEqual(pk, k) {
			return domain.AdminAccess
		}
	}

	user, _ := d.UserByPublicKey(ctx, pk)
	if user != nil {
		return d.AccessLevel(ctx, repo, user.Username)
	}

	return d.AccessLevel(ctx, repo, "")
}

// AccessLevelForUser returns the access level of a user for a repository.
func (d *Backend) AccessLevelForUser(ctx context.Context, repo string, user *domain.User) domain.AccessLevel {
	var username string
	anon := d.AnonAccess(ctx)
	if user != nil {
		username = user.Username
	}

	// If the user is an admin, they have admin access.
	if user != nil && user.Admin {
		return domain.AdminAccess
	}

	// If the repository exists, check if the user is a collaborator.
	r := domain.RepoFromContext(ctx)
	if r == nil {
		r, _ = d.Repository(ctx, repo)
	}

	if r != nil {
		if user != nil {
			// If the user is the owner, they have admin access.
			if r.UserID != nil && *r.UserID == user.ID {
				return domain.AdminAccess
			}
		}

		// If the user is a collaborator, return their access level.
		collabAccess, isCollab, _ := d.IsCollaborator(ctx, repo, username)
		if isCollab {
			if anon > collabAccess {
				return anon
			}
			return collabAccess
		}

		// If the repository is private, the user has no access.
		if r.Private {
			return domain.NoAccess
		}

		// Otherwise, the user has read-only access.
		if user == nil {
			return anon
		}

		return domain.ReadOnlyAccess
	}

	if user != nil {
		// If the repository doesn't exist, the user has read/write access.
		if anon > domain.ReadWriteAccess {
			return anon
		}

		return domain.ReadWriteAccess
	}

	// If the user doesn't exist, give them the anonymous access level.
	return anon
}

// User finds a user by username.
func (d *Backend) User(ctx context.Context, username string) (*domain.User, error) {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return nil, err
	}

	user, err := d.store.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		d.logger.Error("error finding user", "username", username, "error", err)
		return nil, err
	}

	return user, nil
}

// UserByID finds a user by ID.
func (d *Backend) UserByID(ctx context.Context, id int64) (*domain.User, error) {
	user, err := d.store.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		d.logger.Error("error finding user", "id", id, "error", err)
		return nil, err
	}

	return user, nil
}

// UserByPublicKey finds a user by public key.
func (d *Backend) UserByPublicKey(ctx context.Context, pk ssh.PublicKey) (*domain.User, error) {
	user, err := d.store.GetUserByPublicKey(ctx, pk)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		d.logger.Error("error finding user", "pk", sshutils.MarshalAuthorizedKey(pk), "error", err)
		return nil, err
	}

	return user, nil
}

// UserByAccessToken finds a user by access token.
// This also validates the token for expiration and returns domain.ErrTokenExpired.
func (d *Backend) UserByAccessToken(ctx context.Context, token string) (*domain.User, error) {
	token = HashToken(token)

	t, err := d.store.GetAccessTokenByToken(ctx, token)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		d.logger.Error("failed to find user by access token", "err", err, "token", token)
		return nil, err
	}

	if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrTokenExpired
	}

	user, err := d.store.GetUserByAccessToken(ctx, token)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		d.logger.Error("failed to find user by access token", "err", err, "token", token)
		return nil, err
	}

	return user, nil
}

// Users returns all usernames.
func (d *Backend) Users(ctx context.Context) ([]string, error) {
	ms, err := d.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]string, 0, len(ms))
	for _, m := range ms {
		users = append(users, m.Username)
	}

	return users, nil
}

// AddPublicKey adds a public key to a user.
func (d *Backend) AddPublicKey(ctx context.Context, username string, pk ssh.PublicKey) error {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}

	return d.store.AddPublicKeyByUsername(ctx, username, pk)
}

// CreateUser creates a new user.
func (d *Backend) CreateUser(ctx context.Context, username string, opts domain.UserOptions) (*domain.User, error) {
	username = utils.Sanitize(username)
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return nil, err
	}

	if err := d.store.CreateUser(ctx, username, opts.Admin, opts.PublicKeys); err != nil {
		return nil, err
	}

	return d.User(ctx, username)
}

// DeleteUser deletes a user.
func (d *Backend) DeleteUser(ctx context.Context, username string) error {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}

	if err := d.store.DeleteUserByUsername(ctx, username); err != nil {
		return err
	}

	return d.DeleteUserRepositories(ctx, username)
}

// RemovePublicKey removes a public key from a user.
func (d *Backend) RemovePublicKey(ctx context.Context, username string, pk ssh.PublicKey) error {
	return d.store.RemovePublicKeyByUsername(ctx, username, pk)
}

// ListPublicKeys lists the public keys of a user.
func (d *Backend) ListPublicKeys(ctx context.Context, username string) ([]ssh.PublicKey, error) {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return nil, err
	}

	return d.store.ListPublicKeysByUsername(ctx, username)
}

// SetUsername sets the username of a user.
func (d *Backend) SetUsername(ctx context.Context, username string, newUsername string) error {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}

	return d.store.Transaction(ctx, func(tx domain.Store) error {
		user, err := tx.GetUserByUsername(ctx, username)
		if err != nil {
			return err
		}
		user.Username = newUsername
		return tx.UpdateUser(ctx, user)
	})
}

// SetAdmin sets the admin flag of a user.
func (d *Backend) SetAdmin(ctx context.Context, username string, admin bool) error {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}

	return d.store.Transaction(ctx, func(tx domain.Store) error {
		user, err := tx.GetUserByUsername(ctx, username)
		if err != nil {
			return err
		}
		user.Admin = admin
		return tx.UpdateUser(ctx, user)
	})
}

// SetPassword sets the password of a user.
func (d *Backend) SetPassword(ctx context.Context, username string, rawPassword string) error {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}

	password, err := HashPassword(rawPassword)
	if err != nil {
		return err
	}

	user, err := d.store.GetUserByUsername(ctx, username)
	if err != nil {
		return err
	}

	return d.store.SetUserPassword(ctx, user.ID, password)
}
