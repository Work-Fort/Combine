package sqlite

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"

	"golang.org/x/crypto/ssh"

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

func TestIdentityStore(t *testing.T) {
	s := mustOpen(t)
	ctx := context.Background()

	// Upsert creates on first call
	id, err := s.UpsertIdentity(ctx, "uuid-1", "kazw", "Kaz", "user")
	if err != nil {
		t.Fatalf("UpsertIdentity (create): %v", err)
	}
	if id.ID != "uuid-1" {
		t.Errorf("ID = %q, want %q", id.ID, "uuid-1")
	}
	if id.Username != "kazw" {
		t.Errorf("Username = %q, want %q", id.Username, "kazw")
	}
	if id.DisplayName != "Kaz" {
		t.Errorf("DisplayName = %q, want %q", id.DisplayName, "Kaz")
	}
	if id.Type != "user" {
		t.Errorf("Type = %q, want %q", id.Type, "user")
	}

	// Upsert updates on second call
	id2, err := s.UpsertIdentity(ctx, "uuid-1", "kazw-new", "Kaz Walker", "user")
	if err != nil {
		t.Fatalf("UpsertIdentity (update): %v", err)
	}
	if id2.Username != "kazw-new" {
		t.Errorf("Username = %q, want %q", id2.Username, "kazw-new")
	}
	if id2.DisplayName != "Kaz Walker" {
		t.Errorf("DisplayName = %q, want %q", id2.DisplayName, "Kaz Walker")
	}

	// Get by ID
	got, err := s.GetIdentityByID(ctx, "uuid-1")
	if err != nil {
		t.Fatalf("GetIdentityByID: %v", err)
	}
	if got.Username != "kazw-new" {
		t.Errorf("Username = %q, want %q", got.Username, "kazw-new")
	}

	// Get by username
	got, err = s.GetIdentityByUsername(ctx, "kazw-new")
	if err != nil {
		t.Fatalf("GetIdentityByUsername: %v", err)
	}
	if got.ID != "uuid-1" {
		t.Errorf("ID = %q, want %q", got.ID, "uuid-1")
	}

	// Not found
	_, err = s.GetIdentityByID(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrIdentityNotFound) {
		t.Errorf("expected ErrIdentityNotFound, got: %v", err)
	}

	// Admin flag
	if err := s.SetIdentityAdmin(ctx, "uuid-1", true); err != nil {
		t.Fatalf("SetIdentityAdmin: %v", err)
	}
	got, _ = s.GetIdentityByID(ctx, "uuid-1")
	if !got.IsAdmin {
		t.Error("expected IsAdmin = true")
	}

	// List
	ids, err := s.ListIdentities(ctx)
	if err != nil {
		t.Fatalf("ListIdentities: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("ListIdentities returned %d, want 1", len(ids))
	}

	// Public keys
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}

	if err := s.AddIdentityPublicKey(ctx, "uuid-1", sshPub); err != nil {
		t.Fatalf("AddIdentityPublicKey: %v", err)
	}
	keys, err := s.ListIdentityPublicKeys(ctx, "uuid-1")
	if err != nil {
		t.Fatalf("ListIdentityPublicKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("ListIdentityPublicKeys returned %d, want 1", len(keys))
	}

	// Get by public key
	got, err = s.GetIdentityByPublicKey(ctx, sshPub)
	if err != nil {
		t.Fatalf("GetIdentityByPublicKey: %v", err)
	}
	if got.ID != "uuid-1" {
		t.Errorf("ID = %q, want %q", got.ID, "uuid-1")
	}

	// Remove public key
	if err := s.RemoveIdentityPublicKey(ctx, "uuid-1", keys[0].ID); err != nil {
		t.Fatalf("RemoveIdentityPublicKey: %v", err)
	}
	keys, _ = s.ListIdentityPublicKeys(ctx, "uuid-1")
	if len(keys) != 0 {
		t.Errorf("ListIdentityPublicKeys returned %d after remove, want 0", len(keys))
	}
}

func TestCollabStore(t *testing.T) {
	s := mustOpen(t)
	ctx := context.Background()

	// Setup: create identity and repo
	identity, err := s.UpsertIdentity(ctx, "uuid-bob", "bob", "Bob", "user")
	if err != nil {
		t.Fatalf("UpsertIdentity: %v", err)
	}
	repo := &domain.Repo{Name: "collab-repo"}
	if err := s.CreateRepo(ctx, repo); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Add
	if err := s.AddCollabByIdentityAndRepo(ctx, identity.ID, "collab-repo", domain.ReadWriteAccess); err != nil {
		t.Fatalf("AddCollabByIdentityAndRepo: %v", err)
	}

	// Get
	c, err := s.GetCollabByIdentityAndRepo(ctx, identity.ID, "collab-repo")
	if err != nil {
		t.Fatalf("GetCollabByIdentityAndRepo: %v", err)
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

	// List as identities
	identities, err := s.ListCollabsByRepoAsIdentities(ctx, "collab-repo")
	if err != nil {
		t.Fatalf("ListCollabsByRepoAsIdentities: %v", err)
	}
	if len(identities) != 1 || identities[0].Username != "bob" {
		t.Errorf("unexpected identities: %v", identities)
	}

	// Duplicate
	err = s.AddCollabByIdentityAndRepo(ctx, identity.ID, "collab-repo", domain.ReadOnlyAccess)
	if !errors.Is(err, domain.ErrCollaboratorExist) {
		t.Errorf("expected ErrCollaboratorExist, got: %v", err)
	}

	// Remove
	if err := s.RemoveCollabByIdentityAndRepo(ctx, identity.ID, "collab-repo"); err != nil {
		t.Fatalf("RemoveCollabByIdentityAndRepo: %v", err)
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
		_, err := tx.UpsertIdentity(ctx, "uuid-tx", "txuser", "TX User", "user")
		return err
	})
	if err != nil {
		t.Fatalf("Transaction (success): %v", err)
	}
	id, err := s.GetIdentityByID(ctx, "uuid-tx")
	if err != nil {
		t.Fatalf("identity not found after committed tx: %v", err)
	}
	if id.Username != "txuser" {
		t.Errorf("Username = %q, want %q", id.Username, "txuser")
	}

	// Rolled-back transaction
	testErr := errors.New("rollback me")
	err = s.Transaction(ctx, func(tx domain.Store) error {
		if _, err := tx.UpsertIdentity(ctx, "uuid-rollback", "rollbackuser", "Rollback User", "user"); err != nil {
			return err
		}
		return testErr
	})
	if !errors.Is(err, testErr) {
		t.Errorf("expected testErr, got: %v", err)
	}
	_, err = s.GetIdentityByID(ctx, "uuid-rollback")
	if err == nil {
		t.Error("expected error for rolled-back identity, got nil")
	}
}
