package domain

import "errors"

var (
	// ErrNotFound is a generic not-found error.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists is a generic already-exists error.
	ErrAlreadyExists = errors.New("already exists")

	// ErrUnauthorized is returned when the user is not authorized to perform an action.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrRepoNotFound is returned when a repository is not found.
	ErrRepoNotFound = errors.New("repository not found")

	// ErrRepoExist is returned when a repository already exists.
	ErrRepoExist = errors.New("repository already exists")

	// ErrUserNotFound is returned when a user is not found.
	ErrUserNotFound = errors.New("user not found")

	// ErrTokenNotFound is returned when a token is not found.
	ErrTokenNotFound = errors.New("token not found")

	// ErrTokenExpired is returned when a token is expired.
	ErrTokenExpired = errors.New("token expired")

	// ErrCollaboratorNotFound is returned when a collaborator is not found.
	ErrCollaboratorNotFound = errors.New("collaborator not found")

	// ErrCollaboratorExist is returned when a collaborator already exists.
	ErrCollaboratorExist = errors.New("collaborator already exists")

	// ErrFileNotFound is returned when a file is not found.
	ErrFileNotFound = errors.New("file not found")

	// ErrIdentityNotFound is returned when a Passport identity is not found.
	ErrIdentityNotFound = errors.New("identity not found")

	// ErrIssueNotFound is returned when an issue is not found.
	ErrIssueNotFound = errors.New("issue not found")

	// ErrPullRequestNotFound is returned when a pull request is not found.
	ErrPullRequestNotFound = errors.New("pull request not found")

	// ErrReviewNotFound is returned when a review is not found.
	ErrReviewNotFound = errors.New("review not found")
)
