package webhook

import (
	"context"

	"github.com/Work-Fort/Combine/internal/domain"
)

// CollaboratorEvent is a collaborator event.
type CollaboratorEvent struct {
	Common

	// Action is the collaborator event action.
	Action CollaboratorEventAction `json:"action" url:"action"`
	// AccessLevel is the collaborator access level.
	AccessLevel domain.AccessLevel `json:"access_level" url:"access_level"`
	// Collaborator is the collaborator.
	Collaborator User `json:"collaborator" url:"collaborator"`
}

// CollaboratorEventAction is a collaborator event action.
type CollaboratorEventAction string

const (
	// CollaboratorEventAdded is a collaborator added event.
	CollaboratorEventAdded CollaboratorEventAction = "added"
	// CollaboratorEventRemoved is a collaborator removed event.
	CollaboratorEventRemoved CollaboratorEventAction = "removed"
)

// NewCollaboratorEvent sends a collaborator event.
func NewCollaboratorEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, collabIdentityID string, action CollaboratorEventAction) (CollaboratorEvent, error) {
	event := EventCollaborator

	payload := CollaboratorEvent{
		Action: action,
		Common: Common{
			EventType: event,
			Repository: Repository{
				ID:          repo.ID,
				Name:        repo.Name,
				Description: repo.Description,
				ProjectName: repo.ProjectName,
				Private:     repo.Private,
				CreatedAt:   repo.CreatedAt,
				UpdatedAt:   repo.UpdatedAt,
			},
		},
	}

	if identity != nil {
		payload.Sender = User{
			Username: identity.Username,
		}
	}

	datastore := domain.StoreFromContext(ctx)

	// Find repo owner.
	if repo.IdentityID != nil {
		owner, err := datastore.GetIdentityByID(ctx, *repo.IdentityID)
		if err != nil {
			return CollaboratorEvent{}, err
		}
		payload.Repository.Owner.Username = owner.Username
	}

	payload.Repository.DefaultBranch, _ = getDefaultBranch(ctx, repo)

	collab, err := datastore.GetCollabByIdentityAndRepo(ctx, collabIdentityID, repo.Name)
	if err != nil {
		return CollaboratorEvent{}, err
	}

	collabIdentity, err := datastore.GetIdentityByID(ctx, collabIdentityID)
	if err != nil {
		return CollaboratorEvent{}, err
	}

	payload.AccessLevel = collab.AccessLevel
	payload.Collaborator.Username = collabIdentity.Username

	return payload, nil
}
