package domain

import (
	"context"
	"io"

	"golang.org/x/crypto/ssh"

	"github.com/google/uuid"
)

// RepoStore is the port for repository persistence.
type RepoStore interface {
	GetRepoByName(ctx context.Context, name string) (*Repo, error)
	ListRepos(ctx context.Context) ([]*Repo, error)
	ListReposByIdentityID(ctx context.Context, identityID string) ([]*Repo, error)
	CreateRepo(ctx context.Context, repo *Repo) error
	UpdateRepo(ctx context.Context, repo *Repo) error
	DeleteRepoByName(ctx context.Context, name string) error
}

// CollabStore is the port for collaborator persistence.
type CollabStore interface {
	GetCollabByIdentityAndRepo(ctx context.Context, identityID, repo string) (*Collab, error)
	AddCollabByIdentityAndRepo(ctx context.Context, identityID, repo string, level AccessLevel) error
	RemoveCollabByIdentityAndRepo(ctx context.Context, identityID, repo string) error
	ListCollabsByRepo(ctx context.Context, repo string) ([]*Collab, error)
	ListCollabsByRepoAsIdentities(ctx context.Context, repo string) ([]*Identity, error)
}

// SettingStore is the port for settings persistence.
type SettingStore interface {
	GetAnonAccess(ctx context.Context) (AccessLevel, error)
	SetAnonAccess(ctx context.Context, level AccessLevel) error
	GetAllowKeylessAccess(ctx context.Context) (bool, error)
	SetAllowKeylessAccess(ctx context.Context, allow bool) error
}

// LFSStore is the port for Git LFS persistence.
type LFSStore interface {
	CreateLFSObject(ctx context.Context, repoID int64, oid string, size int64) error
	GetLFSObjectByOid(ctx context.Context, repoID int64, oid string) (*LFSObject, error)
	ListLFSObjects(ctx context.Context, repoID int64) ([]*LFSObject, error)
	ListLFSObjectsByName(ctx context.Context, name string) ([]*LFSObject, error)
	DeleteLFSObjectByOid(ctx context.Context, repoID int64, oid string) error

	CreateLFSLockForIdentity(ctx context.Context, repoID int64, identityID, path, refname string) error
	ListLFSLocks(ctx context.Context, repoID int64, page, limit int) ([]*LFSLock, error)
	ListLFSLocksWithCount(ctx context.Context, repoID int64, page, limit int) ([]*LFSLock, int64, error)
	ListLFSLocksForIdentity(ctx context.Context, repoID int64, identityID string) ([]*LFSLock, error)
	GetLFSLockForPath(ctx context.Context, repoID int64, path string) (*LFSLock, error)
	GetLFSLockForIdentityPath(ctx context.Context, repoID int64, identityID, path string) (*LFSLock, error)
	GetLFSLockByID(ctx context.Context, id int64) (*LFSLock, error)
	GetLFSLockForIdentityByID(ctx context.Context, repoID int64, identityID string, id int64) (*LFSLock, error)
	DeleteLFSLock(ctx context.Context, repoID, id int64) error
	DeleteLFSLockForIdentityByID(ctx context.Context, repoID int64, identityID string, id int64) error
}

// WebhookStore is the port for webhook persistence.
type WebhookStore interface {
	GetWebhookByID(ctx context.Context, repoID, id int64) (*Webhook, error)
	ListWebhooksByRepoID(ctx context.Context, repoID int64) ([]*Webhook, error)
	ListWebhooksByRepoIDWhereEvent(ctx context.Context, repoID int64, events []int) ([]*Webhook, error)
	CreateWebhook(ctx context.Context, repoID int64, url string, contentType int, active bool) (int64, error)
	UpdateWebhookByID(ctx context.Context, repoID, id int64, url string, contentType int, active bool) error
	DeleteWebhookByID(ctx context.Context, id int64) error
	DeleteWebhookForRepoByID(ctx context.Context, repoID, id int64) error

	GetWebhookEventByID(ctx context.Context, id int64) (*WebhookEvent, error)
	ListWebhookEventsByWebhookID(ctx context.Context, webhookID int64) ([]*WebhookEvent, error)
	CreateWebhookEvents(ctx context.Context, webhookID int64, events []int) error
	DeleteWebhookEventsByID(ctx context.Context, ids []int64) error

	GetWebhookDeliveryByID(ctx context.Context, webhookID int64, id uuid.UUID) (*WebhookDelivery, error)
	GetWebhookDeliveriesByWebhookID(ctx context.Context, webhookID int64) ([]*WebhookDelivery, error)
	ListWebhookDeliveriesByWebhookID(ctx context.Context, webhookID int64) ([]*WebhookDelivery, error)
	CreateWebhookDelivery(ctx context.Context, id uuid.UUID, webhookID int64, event int, url, method string, requestError error, requestHeaders, requestBody string, responseStatus int, responseHeaders, responseBody string) error
	DeleteWebhookDeliveryByID(ctx context.Context, webhookID int64, id uuid.UUID) error
}

// IdentityStore persists Passport identity records.
type IdentityStore interface {
	UpsertIdentity(ctx context.Context, id, username, displayName, identityType string) (*Identity, error)
	GetIdentityByID(ctx context.Context, id string) (*Identity, error)
	GetIdentityByUsername(ctx context.Context, username string) (*Identity, error)
	GetIdentityByPublicKey(ctx context.Context, pk ssh.PublicKey) (*Identity, error)
	ListIdentities(ctx context.Context) ([]*Identity, error)
	SetIdentityAdmin(ctx context.Context, id string, isAdmin bool) error
	AddIdentityPublicKey(ctx context.Context, identityID string, pk ssh.PublicKey) error
	RemoveIdentityPublicKey(ctx context.Context, identityID string, keyID int64) error
	ListIdentityPublicKeys(ctx context.Context, identityID string) ([]*PublicKey, error)
}

// IssueStore is the port for issue persistence.
type IssueStore interface {
	CreateIssue(ctx context.Context, issue *Issue) error
	GetIssueByNumber(ctx context.Context, repoID, number int64) (*Issue, error)
	ListIssues(ctx context.Context, repoID int64, opts IssueListOptions) ([]*Issue, error)
	UpdateIssue(ctx context.Context, issue *Issue) error

	SetIssueLabels(ctx context.Context, issueID int64, labels []string) error

	CreateIssueComment(ctx context.Context, comment *IssueComment) error
	ListIssueComments(ctx context.Context, issueID int64) ([]*IssueComment, error)
}

// PullRequestStore is the port for pull request persistence.
type PullRequestStore interface {
	CreatePullRequest(ctx context.Context, pr *PullRequest) error
	GetPullRequestByNumber(ctx context.Context, repoID, number int64) (*PullRequest, error)
	ListPullRequests(ctx context.Context, repoID int64, opts PullRequestListOptions) ([]*PullRequest, error)
	UpdatePullRequest(ctx context.Context, pr *PullRequest) error
}

// ReviewStore is the port for pull request review persistence.
type ReviewStore interface {
	CreateReview(ctx context.Context, review *PullRequestReview) error
	ListReviewsByPRID(ctx context.Context, prID int64) ([]*PullRequestReview, error)

	CreateReviewComment(ctx context.Context, comment *ReviewComment) error
	ListReviewComments(ctx context.Context, reviewID int64) ([]*ReviewComment, error)
}

// Store is the composite port for all persistence operations.
type Store interface {
	RepoStore
	CollabStore
	SettingStore
	LFSStore
	WebhookStore
	IdentityStore
	IssueStore
	PullRequestStore
	ReviewStore

	// NOTE: Combine-specific deviation from Nexus/Hive convention.
	// Neither service exposes Transaction on their Store interface.
	// Combine needs this because Backend performs cross-store atomic
	// operations (e.g., DeleteRepository deletes repo + LFS objects).
	Transaction(ctx context.Context, fn func(tx Store) error) error

	Ping(ctx context.Context) error
	io.Closer
}
