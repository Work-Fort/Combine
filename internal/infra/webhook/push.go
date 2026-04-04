package webhook

import (
	"context"
	"fmt"

	gitm "github.com/aymanbagabas/git-module"
	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/git"
	"github.com/Work-Fort/Combine/pkg/config"
)

// PushEvent is a push event.
type PushEvent struct {
	Common

	// Ref is the branch or tag name.
	Ref string `json:"ref" url:"ref"`
	// Before is the previous commit SHA.
	Before string `json:"before" url:"before"`
	// After is the current commit SHA.
	After string `json:"after" url:"after"`
	// Commits is the list of commits.
	Commits []Commit `json:"commits" url:"commits"`
}

// NewPushEvent sends a push event.
func NewPushEvent(ctx context.Context, user *domain.User, repo *domain.Repo, ref, before, after string) (PushEvent, error) {
	event := EventPush

	payload := PushEvent{
		Ref:    ref,
		Before: before,
		After:  after,
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
			return PushEvent{}, err
		}
		payload.Repository.Owner.ID = owner.ID
		payload.Repository.Owner.Username = owner.Username
	}

	// Find commits.
	r, err := openRepoFromContext(ctx, repo.Name)
	if err != nil {
		return PushEvent{}, err
	}

	payload.Repository.DefaultBranch, _ = getDefaultBranch(ctx, repo)

	rev := after
	if !git.IsZeroHash(before) {
		rev = fmt.Sprintf("%s..%s", before, after)
	}

	commits, err := r.Log(rev, gitm.LogOptions{
		// XXX: limit to 20 commits for now
		// TODO: implement a commits api
		MaxCount: 20,
	})
	if err != nil {
		return PushEvent{}, err
	}

	payload.Commits = make([]Commit, len(commits))
	for i, c := range commits {
		payload.Commits[i] = Commit{
			ID:      c.ID.String(),
			Message: c.Message,
			Title:   c.Summary(),
			Author: Author{
				Name:  c.Author.Name,
				Email: c.Author.Email,
				Date:  c.Author.When,
			},
			Committer: Author{
				Name:  c.Committer.Name,
				Email: c.Committer.Email,
				Date:  c.Committer.When,
			},
			Timestamp: c.Committer.When,
		}
	}

	return payload, nil
}
