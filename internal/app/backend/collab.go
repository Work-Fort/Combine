package backend

import (
	"context"
	"errors"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/utils"
	"github.com/Work-Fort/Combine/internal/infra/webhook"
)

// AddCollaborator adds a collaborator to a repository.
func (d *Backend) AddCollaborator(ctx context.Context, repo, identityID string, level domain.AccessLevel) error {
	repo = utils.SanitizeRepo(repo)
	r, err := d.Repository(ctx, repo)
	if err != nil {
		return err
	}

	if err := d.store.AddCollabByIdentityAndRepo(ctx, identityID, repo, level); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return domain.ErrCollaboratorExist
		}
		return err
	}

	identity := domain.IdentityFromContext(ctx)
	wh, err := webhook.NewCollaboratorEvent(ctx, identity, r, identityID, webhook.CollaboratorEventAdded)
	if err != nil {
		return err
	}

	return webhook.SendEvent(ctx, wh)
}

// Collaborators returns a list of collaborator identities for a repository.
func (d *Backend) Collaborators(ctx context.Context, repo string) ([]*domain.Identity, error) {
	repo = utils.SanitizeRepo(repo)
	return d.store.ListCollabsByRepoAsIdentities(ctx, repo)
}

// IsCollaborator returns the access level and true if the identity is a collaborator.
func (d *Backend) IsCollaborator(ctx context.Context, repo, identityID string) (domain.AccessLevel, bool, error) {
	if identityID == "" {
		return -1, false, nil
	}

	repo = utils.SanitizeRepo(repo)
	m, err := d.store.GetCollabByIdentityAndRepo(ctx, identityID, repo)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrCollaboratorNotFound) {
			return -1, false, nil
		}
		return -1, false, err
	}

	return m.AccessLevel, m.ID > 0, nil
}

// RemoveCollaborator removes a collaborator from a repository.
func (d *Backend) RemoveCollaborator(ctx context.Context, repo, identityID string) error {
	repo = utils.SanitizeRepo(repo)
	r, err := d.Repository(ctx, repo)
	if err != nil {
		return err
	}

	identity := domain.IdentityFromContext(ctx)
	wh, err := webhook.NewCollaboratorEvent(ctx, identity, r, identityID, webhook.CollaboratorEventRemoved)
	if err != nil {
		return err
	}

	if err := d.store.RemoveCollabByIdentityAndRepo(ctx, identityID, repo); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrCollaboratorNotFound
		}
		return err
	}

	return webhook.SendEvent(ctx, wh)
}
