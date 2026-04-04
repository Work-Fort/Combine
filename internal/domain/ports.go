package domain

import (
	"context"
	"io"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/google/uuid"
)

// RepoStore is the port for repository persistence.
type RepoStore interface {
	GetRepoByName(ctx context.Context, name string) (*Repo, error)
	ListRepos(ctx context.Context) ([]*Repo, error)
	ListReposByUserID(ctx context.Context, userID int64) ([]*Repo, error)
	CreateRepo(ctx context.Context, repo *Repo) error
	UpdateRepo(ctx context.Context, repo *Repo) error
	DeleteRepoByName(ctx context.Context, name string) error
}

// UserStore is the port for user persistence.
type UserStore interface {
	GetUserByID(ctx context.Context, id int64) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByPublicKey(ctx context.Context, pk ssh.PublicKey) (*User, error)
	GetUserByAccessToken(ctx context.Context, token string) (*User, error)
	ListUsers(ctx context.Context) ([]*User, error)
	CreateUser(ctx context.Context, username string, isAdmin bool, pks []ssh.PublicKey) error
	DeleteUserByUsername(ctx context.Context, username string) error
	UpdateUser(ctx context.Context, user *User) error
	AddPublicKeyByUsername(ctx context.Context, username string, pk ssh.PublicKey) error
	RemovePublicKeyByUsername(ctx context.Context, username string, pk ssh.PublicKey) error
	ListPublicKeysByUserID(ctx context.Context, id int64) ([]ssh.PublicKey, error)
	ListPublicKeysByUsername(ctx context.Context, username string) ([]ssh.PublicKey, error)
	SetUserPassword(ctx context.Context, userID int64, password string) error
}

// CollabStore is the port for collaborator persistence.
type CollabStore interface {
	GetCollabByUsernameAndRepo(ctx context.Context, username string, repo string) (*Collab, error)
	AddCollabByUsernameAndRepo(ctx context.Context, username string, repo string, level AccessLevel) error
	RemoveCollabByUsernameAndRepo(ctx context.Context, username string, repo string) error
	ListCollabsByRepo(ctx context.Context, repo string) ([]*Collab, error)
	ListCollabsByRepoAsUsers(ctx context.Context, repo string) ([]*User, error)
}

// SettingStore is the port for settings persistence.
type SettingStore interface {
	GetAnonAccess(ctx context.Context) (AccessLevel, error)
	SetAnonAccess(ctx context.Context, level AccessLevel) error
	GetAllowKeylessAccess(ctx context.Context) (bool, error)
	SetAllowKeylessAccess(ctx context.Context, allow bool) error
}

// AccessTokenStore is the port for access token persistence.
type AccessTokenStore interface {
	GetAccessToken(ctx context.Context, id int64) (*AccessToken, error)
	GetAccessTokenByToken(ctx context.Context, token string) (*AccessToken, error)
	ListAccessTokensByUserID(ctx context.Context, userID int64) ([]*AccessToken, error)
	CreateAccessToken(ctx context.Context, name string, userID int64, token string, expiresAt time.Time) (*AccessToken, error)
	DeleteAccessToken(ctx context.Context, id int64) error
	DeleteAccessTokenForUser(ctx context.Context, userID int64, id int64) error
}

// LFSStore is the port for Git LFS persistence.
type LFSStore interface {
	CreateLFSObject(ctx context.Context, repoID int64, oid string, size int64) error
	GetLFSObjectByOid(ctx context.Context, repoID int64, oid string) (*LFSObject, error)
	ListLFSObjects(ctx context.Context, repoID int64) ([]*LFSObject, error)
	ListLFSObjectsByName(ctx context.Context, name string) ([]*LFSObject, error)
	DeleteLFSObjectByOid(ctx context.Context, repoID int64, oid string) error

	CreateLFSLockForUser(ctx context.Context, repoID int64, userID int64, path string, refname string) error
	ListLFSLocks(ctx context.Context, repoID int64, page int, limit int) ([]*LFSLock, error)
	ListLFSLocksWithCount(ctx context.Context, repoID int64, page int, limit int) ([]*LFSLock, int64, error)
	ListLFSLocksForUser(ctx context.Context, repoID int64, userID int64) ([]*LFSLock, error)
	GetLFSLockForPath(ctx context.Context, repoID int64, path string) (*LFSLock, error)
	GetLFSLockForUserPath(ctx context.Context, repoID int64, userID int64, path string) (*LFSLock, error)
	GetLFSLockByID(ctx context.Context, id int64) (*LFSLock, error)
	GetLFSLockForUserByID(ctx context.Context, repoID int64, userID int64, id int64) (*LFSLock, error)
	DeleteLFSLock(ctx context.Context, repoID int64, id int64) error
	DeleteLFSLockForUserByID(ctx context.Context, repoID int64, userID int64, id int64) error
}

// WebhookStore is the port for webhook persistence.
type WebhookStore interface {
	GetWebhookByID(ctx context.Context, repoID int64, id int64) (*Webhook, error)
	ListWebhooksByRepoID(ctx context.Context, repoID int64) ([]*Webhook, error)
	ListWebhooksByRepoIDWhereEvent(ctx context.Context, repoID int64, events []int) ([]*Webhook, error)
	CreateWebhook(ctx context.Context, repoID int64, url string, secret string, contentType int, active bool) (int64, error)
	UpdateWebhookByID(ctx context.Context, repoID int64, id int64, url string, secret string, contentType int, active bool) error
	DeleteWebhookByID(ctx context.Context, id int64) error
	DeleteWebhookForRepoByID(ctx context.Context, repoID int64, id int64) error

	GetWebhookEventByID(ctx context.Context, id int64) (*WebhookEvent, error)
	ListWebhookEventsByWebhookID(ctx context.Context, webhookID int64) ([]*WebhookEvent, error)
	CreateWebhookEvents(ctx context.Context, webhookID int64, events []int) error
	DeleteWebhookEventsByID(ctx context.Context, ids []int64) error

	GetWebhookDeliveryByID(ctx context.Context, webhookID int64, id uuid.UUID) (*WebhookDelivery, error)
	GetWebhookDeliveriesByWebhookID(ctx context.Context, webhookID int64) ([]*WebhookDelivery, error)
	ListWebhookDeliveriesByWebhookID(ctx context.Context, webhookID int64) ([]*WebhookDelivery, error)
	CreateWebhookDelivery(ctx context.Context, id uuid.UUID, webhookID int64, event int, url string, method string, requestError error, requestHeaders string, requestBody string, responseStatus int, responseHeaders string, responseBody string) error
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
	GetIssueByNumber(ctx context.Context, repoID int64, number int64) (*Issue, error)
	ListIssues(ctx context.Context, repoID int64, opts IssueListOptions) ([]*Issue, error)
	UpdateIssue(ctx context.Context, issue *Issue) error

	SetIssueLabels(ctx context.Context, issueID int64, labels []string) error

	CreateIssueComment(ctx context.Context, comment *IssueComment) error
	ListIssueComments(ctx context.Context, issueID int64) ([]*IssueComment, error)
}

// Store is the composite port for all persistence operations.
type Store interface {
	RepoStore
	UserStore
	CollabStore
	SettingStore
	AccessTokenStore
	LFSStore
	WebhookStore
	IdentityStore
	IssueStore

	// NOTE: Combine-specific deviation from Nexus/Hive convention.
	// Neither service exposes Transaction on their Store interface.
	// Combine needs this because Backend performs cross-store atomic
	// operations (e.g., DeleteRepository deletes repo + LFS objects).
	Transaction(ctx context.Context, fn func(tx Store) error) error

	Ping(ctx context.Context) error
	io.Closer
}
