package backend

import (
	"context"
	"errors"
	"strings"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/utils"
	"github.com/Work-Fort/Combine/internal/infra/webhook"
)

// AddCollaborator adds a collaborator to a repository.
func (d *Backend) AddCollaborator(ctx context.Context, repo string, username string, level domain.AccessLevel) error {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}

	repo = utils.SanitizeRepo(repo)
	r, err := d.Repository(ctx, repo)
	if err != nil {
		return err
	}

	if err := d.store.AddCollabByUsernameAndRepo(ctx, username, repo, level); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return domain.ErrCollaboratorExist
		}
		return err
	}

	wh, err := webhook.NewCollaboratorEvent(ctx, domain.UserFromContext(ctx), r, username, webhook.CollaboratorEventAdded)
	if err != nil {
		return err
	}

	return webhook.SendEvent(ctx, wh)
}

// Collaborators returns a list of collaborators for a repository.
func (d *Backend) Collaborators(ctx context.Context, repo string) ([]string, error) {
	repo = utils.SanitizeRepo(repo)
	users, err := d.store.ListCollabsByRepoAsUsers(ctx, repo)
	if err != nil {
		return nil, err
	}

	usernames := make([]string, 0, len(users))
	for _, u := range users {
		usernames = append(usernames, u.Username)
	}

	return usernames, nil
}

// IsCollaborator returns the access level and true if the user is a collaborator of the repository.
func (d *Backend) IsCollaborator(ctx context.Context, repo string, username string) (domain.AccessLevel, bool, error) {
	if username == "" {
		return -1, false, nil
	}

	repo = utils.SanitizeRepo(repo)
	m, err := d.store.GetCollabByUsernameAndRepo(ctx, username, repo)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return -1, false, nil
		}
		return -1, false, err
	}

	return m.AccessLevel, m.ID > 0, nil
}

// RemoveCollaborator removes a collaborator from a repository.
func (d *Backend) RemoveCollaborator(ctx context.Context, repo string, username string) error {
	repo = utils.SanitizeRepo(repo)
	r, err := d.Repository(ctx, repo)
	if err != nil {
		return err
	}

	wh, err := webhook.NewCollaboratorEvent(ctx, domain.UserFromContext(ctx), r, username, webhook.CollaboratorEventRemoved)
	if err != nil {
		return err
	}

	if err := d.store.RemoveCollabByUsernameAndRepo(ctx, username, repo); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrCollaboratorNotFound
		}
		return err
	}

	return webhook.SendEvent(ctx, wh)
}
