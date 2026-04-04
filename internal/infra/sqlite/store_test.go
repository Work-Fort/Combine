package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Combine/internal/domain"
)

func mustOpen(t *testing.T) *Store {
	t.Helper()
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRepoStore(t *testing.T) {
	s := mustOpen(t)
	ctx := context.Background()

	// Create
	repo := &domain.Repo{
		Name:        "test-repo",
		ProjectName: "Test Repo",
		Description: "A test repository",
		Private:     true,
	}
	if err := s.CreateRepo(ctx, repo); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	if repo.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	// Get
	got, err := s.GetRepoByName(ctx, "test-repo")
	if err != nil {
		t.Fatalf("GetRepoByName: %v", err)
	}
	if got.Name != "test-repo" {
		t.Errorf("Name = %q, want %q", got.Name, "test-repo")
	}
	if got.ProjectName != "Test Repo" {
		t.Errorf("ProjectName = %q, want %q", got.ProjectName, "Test Repo")
	}
	if !got.Private {
		t.Error("expected Private = true")
	}

	// Not found
	_, err = s.GetRepoByName(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrRepoNotFound) {
		t.Errorf("expected ErrRepoNotFound, got: %v", err)
	}

	// Duplicate
	dup := &domain.Repo{Name: "test-repo"}
	err = s.CreateRepo(ctx, dup)
	if !errors.Is(err, domain.ErrRepoExist) {
		t.Errorf("expected ErrRepoExist, got: %v", err)
	}

	// List
	repos, err := s.ListRepos(ctx)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("ListRepos returned %d repos, want 1", len(repos))
	}

	// Update
	got.Description = "updated"
	got.Private = false
	if err := s.UpdateRepo(ctx, got); err != nil {
		t.Fatalf("UpdateRepo: %v", err)
	}
	updated, _ := s.GetRepoByName(ctx, "test-repo")
	if updated.Description != "updated" {
		t.Errorf("Description = %q, want %q", updated.Description, "updated")
	}
	if updated.Private {
		t.Error("expected Private = false after update")
	}

	// Delete
	if err := s.DeleteRepoByName(ctx, "test-repo"); err != nil {
		t.Fatalf("DeleteRepoByName: %v", err)
	}
	repos, _ = s.ListRepos(ctx)
	if len(repos) != 0 {
		t.Errorf("ListRepos returned %d repos after delete, want 0", len(repos))
	}
}

func TestUserStore(t *testing.T) {
	s := mustOpen(t)
	ctx := context.Background()

	// Create
	if err := s.CreateUser(ctx, "alice", true, nil); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Get by username
	u, err := s.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("Username = %q, want %q", u.Username, "alice")
	}
	if !u.Admin {
		t.Error("expected Admin = true")
	}

	// Get by ID
	u2, err := s.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if u2.Username != "alice" {
		t.Errorf("Username = %q, want %q", u2.Username, "alice")
	}

	// Not found
	_, err = s.GetUserByUsername(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got: %v", err)
	}

	// List
	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("ListUsers returned %d users, want 1", len(users))
	}

	// Update
	u.Admin = false
	if err := s.UpdateUser(ctx, u); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	u3, _ := s.GetUserByID(ctx, u.ID)
	if u3.Admin {
		t.Error("expected Admin = false after update")
	}

	// Password
	if err := s.SetUserPassword(ctx, u.ID, "secret"); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	u4, _ := s.GetUserByID(ctx, u.ID)
	if u4.Password != "secret" {
		t.Errorf("Password = %q, want %q", u4.Password, "secret")
	}

	// Delete
	if err := s.DeleteUserByUsername(ctx, "alice"); err != nil {
		t.Fatalf("DeleteUserByUsername: %v", err)
	}
	users, _ = s.ListUsers(ctx)
	if len(users) != 0 {
		t.Errorf("ListUsers returned %d users after delete, want 0", len(users))
	}
}

func TestCollabStore(t *testing.T) {
	s := mustOpen(t)
	ctx := context.Background()

	// Setup: create user and repo
	if err := s.CreateUser(ctx, "bob", false, nil); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	repo := &domain.Repo{Name: "collab-repo"}
	if err := s.CreateRepo(ctx, repo); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Add
	if err := s.AddCollabByUsernameAndRepo(ctx, "bob", "collab-repo", domain.ReadWriteAccess); err != nil {
		t.Fatalf("AddCollabByUsernameAndRepo: %v", err)
	}

	// Get
	c, err := s.GetCollabByUsernameAndRepo(ctx, "bob", "collab-repo")
	if err != nil {
		t.Fatalf("GetCollabByUsernameAndRepo: %v", err)
	}
	if c.AccessLevel != domain.ReadWriteAccess {
		t.Errorf("AccessLevel = %v, want %v", c.AccessLevel, domain.ReadWriteAccess)
	}

	// List
	collabs, err := s.ListCollabsByRepo(ctx, "collab-repo")
	if err != nil {
		t.Fatalf("ListCollabsByRepo: %v", err)
	}
	if len(collabs) != 1 {
		t.Errorf("ListCollabsByRepo returned %d, want 1", len(collabs))
	}

	// List as users
	users, err := s.ListCollabsByRepoAsUsers(ctx, "collab-repo")
	if err != nil {
		t.Fatalf("ListCollabsByRepoAsUsers: %v", err)
	}
	if len(users) != 1 || users[0].Username != "bob" {
		t.Errorf("unexpected users: %v", users)
	}

	// Duplicate
	err = s.AddCollabByUsernameAndRepo(ctx, "bob", "collab-repo", domain.ReadOnlyAccess)
	if !errors.Is(err, domain.ErrCollaboratorExist) {
		t.Errorf("expected ErrCollaboratorExist, got: %v", err)
	}

	// Remove
	if err := s.RemoveCollabByUsernameAndRepo(ctx, "bob", "collab-repo"); err != nil {
		t.Fatalf("RemoveCollabByUsernameAndRepo: %v", err)
	}
	collabs, _ = s.ListCollabsByRepo(ctx, "collab-repo")
	if len(collabs) != 0 {
		t.Errorf("ListCollabsByRepo returned %d after remove, want 0", len(collabs))
	}
}

func TestSettingStore(t *testing.T) {
	s := mustOpen(t)
	ctx := context.Background()

	// Default anon access
	level, err := s.GetAnonAccess(ctx)
	if err != nil {
		t.Fatalf("GetAnonAccess: %v", err)
	}
	if level != domain.ReadOnlyAccess {
		t.Errorf("AnonAccess = %v, want %v", level, domain.ReadOnlyAccess)
	}

	// Set anon access
	if err := s.SetAnonAccess(ctx, domain.NoAccess); err != nil {
		t.Fatalf("SetAnonAccess: %v", err)
	}
	level, _ = s.GetAnonAccess(ctx)
	if level != domain.NoAccess {
		t.Errorf("AnonAccess = %v after set, want %v", level, domain.NoAccess)
	}

	// Default allow keyless
	allow, err := s.GetAllowKeylessAccess(ctx)
	if err != nil {
		t.Fatalf("GetAllowKeylessAccess: %v", err)
	}
	if !allow {
		t.Error("expected AllowKeyless = true by default")
	}

	// Set allow keyless
	if err := s.SetAllowKeylessAccess(ctx, false); err != nil {
		t.Fatalf("SetAllowKeylessAccess: %v", err)
	}
	allow, _ = s.GetAllowKeylessAccess(ctx)
	if allow {
		t.Error("expected AllowKeyless = false after set")
	}
}

func TestTransaction(t *testing.T) {
	s := mustOpen(t)
	ctx := context.Background()

	// Successful transaction
	err := s.Transaction(ctx, func(tx domain.Store) error {
		return tx.CreateUser(ctx, "txuser", false, nil)
	})
	if err != nil {
		t.Fatalf("Transaction (success): %v", err)
	}
	u, err := s.GetUserByUsername(ctx, "txuser")
	if err != nil {
		t.Fatalf("user not found after committed tx: %v", err)
	}
	if u.Username != "txuser" {
		t.Errorf("Username = %q, want %q", u.Username, "txuser")
	}

	// Rolled-back transaction
	testErr := errors.New("rollback me")
	err = s.Transaction(ctx, func(tx domain.Store) error {
		if err := tx.CreateUser(ctx, "rollbackuser", false, nil); err != nil {
			return err
		}
		return testErr
	})
	if !errors.Is(err, testErr) {
		t.Errorf("expected testErr, got: %v", err)
	}
	_, err = s.GetUserByUsername(ctx, "rollbackuser")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound for rolled-back user, got: %v", err)
	}
}
