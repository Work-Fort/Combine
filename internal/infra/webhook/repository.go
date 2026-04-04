package webhook

import (
	"context"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/legacy/config"
)

// RepositoryEvent is a repository payload.
type RepositoryEvent struct {
	Common

	// Action is the repository event action.
	Action RepositoryEventAction `json:"action" url:"action"`
}

// RepositoryEventAction is a repository event action.
type RepositoryEventAction string

const (
	// RepositoryEventActionDelete is a repository deleted event.
	RepositoryEventActionDelete RepositoryEventAction = "delete"
	// RepositoryEventActionRename is a repository renamed event.
	RepositoryEventActionRename RepositoryEventAction = "rename"
	// RepositoryEventActionVisibilityChange is a repository visibility changed event.
	RepositoryEventActionVisibilityChange RepositoryEventAction = "visibility_change"
	// RepositoryEventActionDefaultBranchChange is a repository default branch changed event.
	RepositoryEventActionDefaultBranchChange RepositoryEventAction = "default_branch_change"
)

// NewRepositoryEvent sends a repository event.
func NewRepositoryEvent(ctx context.Context, user *domain.User, repo *domain.Repo, action RepositoryEventAction) (RepositoryEvent, error) {
	var event Event
	switch action {
	case RepositoryEventActionVisibilityChange:
		event = EventRepositoryVisibilityChange
	default:
		event = EventRepository
	}

	payload := RepositoryEvent{
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
			Sender: User{
				ID:       user.ID,
				Username: user.Username,
			},
		},
	}

	cfg := config.FromContext(ctx)
	payload.Repository.HTTPURL = repoURL(cfg.HTTP.PublicURL, repo.Name)
	payload.Repository.SSHURL = repoURL(cfg.SSH.PublicURL, repo.Name)

	// Find repo owner.
	if repo.UserID != nil {
		datastore := domain.StoreFromContext(ctx)
		owner, err := datastore.GetUserByID(ctx, *repo.UserID)
		if err != nil {
			return RepositoryEvent{}, err
		}
		payload.Repository.Owner.ID = owner.ID
		payload.Repository.Owner.Username = owner.Username
	}

	payload.Repository.DefaultBranch, _ = getDefaultBranch(ctx, repo)

	return payload, nil
}
