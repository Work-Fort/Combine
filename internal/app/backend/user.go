package backend

import (
	"context"
	"errors"

	"golang.org/x/crypto/ssh"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/sshutils"
)

// AccessLevel returns the access level of an identity (by username) for a repository.
func (d *Backend) AccessLevel(ctx context.Context, repo, username string) domain.AccessLevel {
	identity, _ := d.IdentityByUsername(ctx, username)
	return d.AccessLevelForIdentity(ctx, repo, identity)
}

// AccessLevelByPublicKey returns the access level of a public key for a repository.
func (d *Backend) AccessLevelByPublicKey(ctx context.Context, repo string, pk ssh.PublicKey) domain.AccessLevel {
	for _, k := range d.cfg.AdminKeys {
		if sshutils.KeysEqual(pk, k) {
			return domain.AdminAccess
		}
	}

	identity, _ := d.store.GetIdentityByPublicKey(ctx, pk)
	if identity != nil {
		return d.AccessLevelForIdentity(ctx, repo, identity)
	}

	return d.AccessLevelForIdentity(ctx, repo, nil)
}

// AccessLevelForIdentity returns the access level of an identity for a repository.
func (d *Backend) AccessLevelForIdentity(ctx context.Context, repo string, identity *domain.Identity) domain.AccessLevel {
	anon := d.AnonAccess(ctx)

	if identity != nil && identity.IsAdmin {
		return domain.AdminAccess
	}

	r := domain.RepoFromContext(ctx)
	if r == nil {
		r, _ = d.Repository(ctx, repo)
	}

	if r != nil {
		if identity != nil {
			if r.IdentityID != nil && *r.IdentityID == identity.ID {
				return domain.AdminAccess
			}
		}

		var identityID string
		if identity != nil {
			identityID = identity.ID
		}
		collabAccess, isCollab, _ := d.IsCollaborator(ctx, repo, identityID)
		if isCollab {
			if anon > collabAccess {
				return anon
			}
			return collabAccess
		}

		if r.Private {
			return domain.NoAccess
		}

		if identity == nil {
			return anon
		}

		return domain.ReadOnlyAccess
	}

	if identity != nil {
		if anon > domain.ReadWriteAccess {
			return anon
		}
		return domain.ReadWriteAccess
	}

	return anon
}

// IdentityByUsername finds an identity by username.
func (d *Backend) IdentityByUsername(ctx context.Context, username string) (*domain.Identity, error) {
	identity, err := d.store.GetIdentityByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		d.logger.Error("error finding identity", "username", username, "error", err)
		return nil, err
	}
	return identity, nil
}

// IdentityByPublicKey finds an identity by public key.
func (d *Backend) IdentityByPublicKey(ctx context.Context, pk ssh.PublicKey) (*domain.Identity, error) {
	identity, err := d.store.GetIdentityByPublicKey(ctx, pk)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		d.logger.Error("error finding identity", "pk", sshutils.MarshalAuthorizedKey(pk), "error", err)
		return nil, err
	}
	return identity, nil
}
