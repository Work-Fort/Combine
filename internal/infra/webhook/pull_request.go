package webhook

import (
	"context"
	"time"

	"github.com/Work-Fort/Combine/internal/config"
	"github.com/Work-Fort/Combine/internal/domain"
)

// PullRequestPayload is the PR representation in webhook payloads.
type PullRequestPayload struct {
	Number       int64      `json:"number"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	SourceBranch string     `json:"source_branch"`
	TargetBranch string     `json:"target_branch"`
	Status       string     `json:"status"`
	MergeMethod  *string    `json:"merge_method,omitempty"`
	Author       User       `json:"author"`
	MergedBy     *User      `json:"merged_by,omitempty"`
	Assignee     *User      `json:"assignee,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	MergedAt     *time.Time `json:"merged_at,omitempty"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
}

// PullRequestOpenedEvent is fired when a pull request is created.
type PullRequestOpenedEvent struct {
	Common
	Sender      IdentitySender     `json:"sender"`
	PullRequest PullRequestPayload `json:"pull_request"`
}

// PullRequestClosedEvent is fired when a pull request is closed without merging.
type PullRequestClosedEvent struct {
	Common
	Sender      IdentitySender     `json:"sender"`
	PullRequest PullRequestPayload `json:"pull_request"`
}

// PullRequestMergedEvent is fired when a pull request is merged.
type PullRequestMergedEvent struct {
	Common
	Sender      IdentitySender     `json:"sender"`
	PullRequest PullRequestPayload `json:"pull_request"`
}

func buildPRCommon(ctx context.Context, repo *domain.Repo, event Event) Common {
	cfg := config.FromContext(ctx)
	c := Common{
		EventType: event,
		Repository: Repository{
			ID:          repo.ID,
			Name:        repo.Name,
			Description: repo.Description,
			ProjectName: repo.ProjectName,
			Private:     repo.Private,
			HTTPURL:     repoURL(cfg.HTTP.PublicURL, repo.Name),
			SSHURL:      repoURL(cfg.SSH.PublicURL, repo.Name),
			CreatedAt:   repo.CreatedAt,
			UpdatedAt:   repo.UpdatedAt,
		},
	}
	c.Repository.DefaultBranch, _ = getDefaultBranch(ctx, repo)
	return c
}

func buildPRPayload(pr *domain.PullRequest) PullRequestPayload {
	p := PullRequestPayload{
		Number:       pr.Number,
		Title:        pr.Title,
		Body:         pr.Body,
		SourceBranch: pr.SourceBranch,
		TargetBranch: pr.TargetBranch,
		Status:       string(pr.Status),
		CreatedAt:    pr.CreatedAt,
		UpdatedAt:    pr.UpdatedAt,
		MergedAt:     pr.MergedAt,
		ClosedAt:     pr.ClosedAt,
	}
	if pr.MergeMethod != nil {
		mm := string(*pr.MergeMethod)
		p.MergeMethod = &mm
	}
	return p
}

// NewPullRequestOpenedEvent creates a new pull request opened event.
func NewPullRequestOpenedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, pr *domain.PullRequest) (PullRequestOpenedEvent, error) {
	return PullRequestOpenedEvent{
		Common:      buildPRCommon(ctx, repo, EventPullRequestOpened),
		Sender:      identitySender(identity),
		PullRequest: buildPRPayload(pr),
	}, nil
}

// NewPullRequestClosedEvent creates a new pull request closed event.
func NewPullRequestClosedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, pr *domain.PullRequest) (PullRequestClosedEvent, error) {
	return PullRequestClosedEvent{
		Common:      buildPRCommon(ctx, repo, EventPullRequestClosed),
		Sender:      identitySender(identity),
		PullRequest: buildPRPayload(pr),
	}, nil
}

// NewPullRequestMergedEvent creates a new pull request merged event.
func NewPullRequestMergedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, pr *domain.PullRequest) (PullRequestMergedEvent, error) {
	return PullRequestMergedEvent{
		Common:      buildPRCommon(ctx, repo, EventPullRequestMerged),
		Sender:      identitySender(identity),
		PullRequest: buildPRPayload(pr),
	}, nil
}
