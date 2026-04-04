package domain

import (
	"encoding"
	"errors"
	"time"

	"golang.org/x/crypto/ssh"

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
	UserID      *int64
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

// User represents a user.
type User struct {
	ID        int64
	Username  string
	Admin     bool
	Password  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserOptions are options for creating a user.
type UserOptions struct {
	Admin      bool
	PublicKeys []ssh.PublicKey
}

// PublicKey represents a public key.
type PublicKey struct {
	ID        int64
	UserID    int64
	PublicKey string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Collab represents a repository collaborator.
type Collab struct {
	ID          int64
	RepoID      int64
	UserID      int64
	AccessLevel AccessLevel
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AccessToken represents an access token.
type AccessToken struct {
	ID        int64
	Name      string
	UserID    int64
	Token     string
	ExpiresAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
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
	ID        int64
	Path      string
	UserID    int64
	RepoID    int64
	Refname   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Identity represents a Passport-authenticated identity stored locally.
type Identity struct {
	ID          string    // Passport UUID, primary key
	Username    string    // From Passport claims
	DisplayName string    // From Passport claims
	Type        string    // "user", "agent", "service"
	IsAdmin     bool      // Local admin flag
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const (
	IdentityTypeUser    = "user"
	IdentityTypeAgent   = "agent"
	IdentityTypeService = "service"
)

// Webhook is a repository webhook.
type Webhook struct {
	ID          int64
	RepoID      int64
	URL         string
	Secret      string
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
