package webhook

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/git"
	"github.com/Work-Fort/Combine/internal/config"
)

// BranchTagEvent is a branch or tag event.
type BranchTagEvent struct {
	Common

	// Ref is the branch or tag name.
	Ref string `json:"ref" url:"ref"`
	// Before is the previous commit SHA.
	Before string `json:"before" url:"before"`
	// After is the current commit SHA.
	After string `json:"after" url:"after"`
	// Created is whether the branch or tag was created.
	Created bool `json:"created" url:"created"`
	// Deleted is whether the branch or tag was deleted.
	Deleted bool `json:"deleted" url:"deleted"`
}

// NewBranchTagEvent sends a branch or tag event.
func NewBranchTagEvent(ctx context.Context, user *domain.User, repo *domain.Repo, ref, before, after string) (BranchTagEvent, error) {
	var event Event
	if git.IsZeroHash(before) {
		event = EventBranchTagCreate
	} else if git.IsZeroHash(after) {
		event = EventBranchTagDelete
	} else {
		return BranchTagEvent{}, fmt.Errorf("invalid branch or tag event: before=%q after=%q", before, after)
	}

	payload := BranchTagEvent{
		Ref:     ref,
		Before:  before,
		After:   after,
		Created: git.IsZeroHash(before),
		Deleted: git.IsZeroHash(after),
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

	if user != nil {
		payload.Sender = User{
			ID:       user.ID,
			Username: user.Username,
		}
	}

	cfg := config.FromContext(ctx)
	payload.Repository.HTTPURL = repoURL(cfg.HTTP.PublicURL, repo.Name)
	payload.Repository.SSHURL = repoURL(cfg.SSH.PublicURL, repo.Name)

	// Find repo owner.
	if repo.UserID != nil {
		datastore := domain.StoreFromContext(ctx)
		owner, err := datastore.GetUserByID(ctx, *repo.UserID)
		if err != nil {
			return BranchTagEvent{}, err
		}
		payload.Repository.Owner.ID = owner.ID
		payload.Repository.Owner.Username = owner.Username
	}

	payload.Repository.DefaultBranch, _ = getDefaultBranch(ctx, repo)

	return payload, nil
}
