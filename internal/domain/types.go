package domain

import (
	"encoding"
	"errors"
	"time"

	"github.com/google/uuid"
)

// AccessLevel is the level of access allowed to a repo.
type AccessLevel int

const (
	// NoAccess does not allow access to the repo.
	NoAccess AccessLevel = iota

	// ReadOnlyAccess allows read-only access to the repo.
	ReadOnlyAccess

	// ReadWriteAccess allows read and write access to the repo.
	ReadWriteAccess

	// AdminAccess allows read, write, and admin access to the repo.
	AdminAccess
)

// String returns the string representation of the access level.
func (a AccessLevel) String() string {
	switch a {
	case NoAccess:
		return "no-access"
	case ReadOnlyAccess:
		return "read-only"
	case ReadWriteAccess:
		return "read-write"
	case AdminAccess:
		return "admin-access"
	default:
		return "unknown"
	}
}

// ParseAccessLevel parses an access level string.
func ParseAccessLevel(s string) AccessLevel {
	switch s {
	case "no-access":
		return NoAccess
	case "read-only":
		return ReadOnlyAccess
	case "read-write":
		return ReadWriteAccess
	case "admin-access":
		return AdminAccess
	default:
		return AccessLevel(-1)
	}
}

var (
	_ encoding.TextMarshaler   = AccessLevel(0)
	_ encoding.TextUnmarshaler = (*AccessLevel)(nil)
)

// MarshalText implements encoding.TextMarshaler.
func (a AccessLevel) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (a *AccessLevel) UnmarshalText(text []byte) error {
	l := ParseAccessLevel(string(text))
	if l < 0 {
		return ErrInvalidAccessLevel
	}
	*a = l
	return nil
}

// ErrInvalidAccessLevel is returned when an invalid access level is provided.
var ErrInvalidAccessLevel = errors.New("invalid access level")

// Repo is a Git repository.
type Repo struct {
	ID          int64
	Name        string
	ProjectName string
	Description string
	Private     bool
	Mirror      bool
	Hidden      bool
	IdentityID  *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RepoOptions are options for creating or updating a repository.
type RepoOptions struct {
	Private     bool
	Description string
	ProjectName string
	Mirror      bool
	Hidden      bool
	LFS         bool
	LFSEndpoint string
}

// PublicKey represents an SSH public key belonging to an identity.
type PublicKey struct {
	ID        int64
	PublicKey string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Collab represents a repository collaborator.
type Collab struct {
	ID          int64
	RepoID      int64
	IdentityID  string
	AccessLevel AccessLevel
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Settings represents a settings record.
type Settings struct {
	ID        int64
	Key       string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// LFSObject is a Git LFS object.
type LFSObject struct {
	ID        int64
	Oid       string
	Size      int64
	RepoID    int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// LFSLock is a Git LFS lock.
type LFSLock struct {
	ID         int64
	Path       string
	IdentityID string
	RepoID     int64
	Refname    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Identity represents a Passport-authenticated identity stored locally.
type Identity struct {
	ID          string // Passport UUID, primary key
	Username    string // From Passport claims
	DisplayName string // From Passport claims
	Type        string // "user", "agent", "service"
	IsAdmin     bool   // Local admin flag
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const (
	IdentityTypeUser    = "user"
	IdentityTypeAgent   = "agent"
	IdentityTypeService = "service"
)

// IssueStatus represents the status of an issue.
type IssueStatus string

const (
	IssueStatusOpen       IssueStatus = "open"
	IssueStatusInProgress IssueStatus = "in_progress"
	IssueStatusClosed     IssueStatus = "closed"
)

// IssueResolution represents the resolution of a closed issue.
type IssueResolution string

const (
	IssueResolutionNone      IssueResolution = ""
	IssueResolutionFixed     IssueResolution = "fixed"
	IssueResolutionWontfix   IssueResolution = "wontfix"
	IssueResolutionDuplicate IssueResolution = "duplicate"
)

// Issue is a repository issue.
type Issue struct {
	ID         int64  // Global autoincrement PK (internal)
	Number     int64  // Per-repo issue number (user-facing)
	RepoID     int64  // FK to repos.id
	AuthorID   string // FK to identities.id
	Title      string
	Body       string
	Status     IssueStatus
	Resolution IssueResolution
	AssigneeID *string  // FK to identities.id, nullable
	Labels     []string // Denormalized from issue_labels table
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ClosedAt   *time.Time
}

// IssueComment is a comment on an issue.
type IssueComment struct {
	ID        int64
	IssueID   int64  // FK to issues.id (global PK)
	AuthorID  string // FK to identities.id
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PullRequestStatus represents the status of a pull request.
type PullRequestStatus string

const (
	PullRequestStatusOpen   PullRequestStatus = "open"
	PullRequestStatusMerged PullRequestStatus = "merged"
	PullRequestStatusClosed PullRequestStatus = "closed"
)

// MergeMethod represents a pull request merge strategy.
type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "merge"
	MergeMethodSquash MergeMethod = "squash"
	MergeMethodRebase MergeMethod = "rebase"
)

// PullRequest is a repository pull request.
type PullRequest struct {
	ID           int64  // Global autoincrement PK (internal)
	Number       int64  // Per-repo number (shared with issues)
	RepoID       int64  // FK to repos.id
	AuthorID     string // FK to identities.id
	Title        string
	Body         string
	SourceBranch string
	TargetBranch string
	Status       PullRequestStatus
	MergeMethod  *MergeMethod // Set when merged
	MergedBy     *string      // FK to identities.id, nullable
	AssigneeID   *string      // FK to identities.id, nullable
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MergedAt     *time.Time
	ClosedAt     *time.Time
}

// PullRequestListOptions controls filtering for ListPullRequests.
type PullRequestListOptions struct {
	Status   *PullRequestStatus
	AuthorID *string
}

// ReviewState represents the state of a pull request review.
type ReviewState string

const (
	ReviewStatePending          ReviewState = "pending"
	ReviewStateApproved         ReviewState = "approved"
	ReviewStateChangesRequested ReviewState = "changes_requested"
	ReviewStateCommented        ReviewState = "commented"
)

// PullRequestReview is a review on a pull request.
type PullRequestReview struct {
	ID        int64  // Global PK
	PRID      int64  // FK to pull_requests.id
	AuthorID  string // FK to identities.id
	State     ReviewState
	Body      string
	Comments  []*ReviewComment // Populated on read
	CreatedAt time.Time
}

// ReviewComment is a line-level comment on a file in a PR review.
type ReviewComment struct {
	ID        int64
	ReviewID  int64  // FK to pull_request_reviews.id
	Path      string // File path in diff
	Line      int    // Line number
	Side      string // "left" or "right"
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IssueListOptions controls filtering for ListIssues.
type IssueListOptions struct {
	Status     *IssueStatus
	Label      *string
	AssigneeID *string
}

// Webhook is a repository webhook.
type Webhook struct {
	ID          int64
	RepoID      int64
	URL         string
	ContentType int
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WebhookEvent is a webhook event.
type WebhookEvent struct {
	ID        int64
	WebhookID int64
	Event     int
	CreatedAt time.Time
}

// WebhookDelivery is a webhook delivery.
type WebhookDelivery struct {
	ID              uuid.UUID
	WebhookID       int64
	Event           int
	RequestURL      string
	RequestMethod   string
	RequestError    string
	RequestHeaders  string
	RequestBody     string
	ResponseStatus  int
	ResponseHeaders string
	ResponseBody    string
	CreatedAt       time.Time
}
