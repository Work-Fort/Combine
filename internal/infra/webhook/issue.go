package webhook

import (
	"context"
	"time"

	"github.com/Work-Fort/Combine/internal/config"
	"github.com/Work-Fort/Combine/internal/domain"
)

// IssuePayload is the issue representation in webhook payloads.
type IssuePayload struct {
	Number     int64      `json:"number"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Status     string     `json:"status"`
	Resolution string     `json:"resolution"`
	Author     User       `json:"author"`
	Assignee   *User      `json:"assignee,omitempty"`
	Labels     []string   `json:"labels"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
}

// CommentPayload is the comment representation in webhook payloads.
type CommentPayload struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	Author    User      `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// IdentitySender represents a Passport identity in webhook payloads.
type IdentitySender struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// IssueOpenedEvent is fired when a new issue is created.
type IssueOpenedEvent struct {
	Common
	Sender IdentitySender `json:"sender"`
	Issue  IssuePayload   `json:"issue"`
}

// IssueStatusChangedEvent is fired when an issue's status changes.
type IssueStatusChangedEvent struct {
	Common
	Sender    IdentitySender `json:"sender"`
	Issue     IssuePayload   `json:"issue"`
	OldStatus string         `json:"old_status"`
	NewStatus string         `json:"new_status"`
}

// IssueClosedEvent is fired when an issue is closed.
type IssueClosedEvent struct {
	Common
	Sender     IdentitySender `json:"sender"`
	Issue      IssuePayload   `json:"issue"`
	Resolution string         `json:"resolution"`
}

// IssueCommentEvent is fired when a comment is added to an issue.
type IssueCommentEvent struct {
	Common
	Sender  IdentitySender `json:"sender"`
	Issue   IssuePayload   `json:"issue"`
	Comment CommentPayload `json:"comment"`
}

func buildIssueCommon(ctx context.Context, repo *domain.Repo, event Event) Common {
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

func buildIssuePayload(issue *domain.Issue) IssuePayload {
	p := IssuePayload{
		Number:     issue.Number,
		Title:      issue.Title,
		Body:       issue.Body,
		Status:     string(issue.Status),
		Resolution: string(issue.Resolution),
		Labels:     issue.Labels,
		CreatedAt:  issue.CreatedAt,
		UpdatedAt:  issue.UpdatedAt,
		ClosedAt:   issue.ClosedAt,
	}
	if p.Labels == nil {
		p.Labels = []string{}
	}
	return p
}

func identitySender(identity *domain.Identity) IdentitySender {
	if identity == nil {
		return IdentitySender{}
	}
	return IdentitySender{ID: identity.ID, Username: identity.Username}
}

// NewIssueOpenedEvent creates a new issue opened event.
func NewIssueOpenedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, issue *domain.Issue) (IssueOpenedEvent, error) {
	return IssueOpenedEvent{
		Common: buildIssueCommon(ctx, repo, EventIssueOpened),
		Sender: identitySender(identity),
		Issue:  buildIssuePayload(issue),
	}, nil
}

// NewIssueStatusChangedEvent creates a new issue status changed event.
func NewIssueStatusChangedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, issue *domain.Issue, oldStatus, newStatus string) (IssueStatusChangedEvent, error) {
	return IssueStatusChangedEvent{
		Common:    buildIssueCommon(ctx, repo, EventIssueStatusChanged),
		Sender:    identitySender(identity),
		Issue:     buildIssuePayload(issue),
		OldStatus: oldStatus,
		NewStatus: newStatus,
	}, nil
}

// NewIssueClosedEvent creates a new issue closed event.
func NewIssueClosedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, issue *domain.Issue, resolution string) (IssueClosedEvent, error) {
	return IssueClosedEvent{
		Common:     buildIssueCommon(ctx, repo, EventIssueClosed),
		Sender:     identitySender(identity),
		Issue:      buildIssuePayload(issue),
		Resolution: resolution,
	}, nil
}

// NewIssueCommentEvent creates a new issue comment event.
func NewIssueCommentEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, issue *domain.Issue, comment *domain.IssueComment) (IssueCommentEvent, error) {
	return IssueCommentEvent{
		Common:  buildIssueCommon(ctx, repo, EventIssueComment),
		Sender:  identitySender(identity),
		Issue:   buildIssuePayload(issue),
		Comment: CommentPayload{
			ID:        comment.ID,
			Body:      comment.Body,
			Author:    User{Username: identity.Username},
			CreatedAt: comment.CreatedAt,
		},
	}, nil
}
