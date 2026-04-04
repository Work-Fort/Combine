package ssh

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"charm.land/log/v2"
	"github.com/charmbracelet/keygen"
	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/domain"
	infra "github.com/Work-Fort/Combine/internal/infra"
	"github.com/charmbracelet/ssh"
	"github.com/matryer/is"
	gossh "golang.org/x/crypto/ssh"
)

// TestAuthenticationBypass tests for CVE-TBD: Authentication Bypass Vulnerability
func TestAuthenticationBypass(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	// Setup temporary database
	dp := t.TempDir()

	store, err := infra.Open(filepath.Join(dp, "test.db"))
	is.NoErr(err)
	defer store.Close()

	ctx = domain.WithStoreContext(ctx, store)
	logger := log.Default()
	beCfg := backend.BackendConfig{
		RepoDir: filepath.Join(dp, "repos"),
		DataDir: dp,
	}
	be := backend.New(ctx, store, beCfg, logger)
	ctx = backend.WithContext(ctx, be)

	// Generate keys for admin and attacker
	adminKeyPath := dp + "/admin_key"
	adminPair, err := keygen.New(adminKeyPath, keygen.WithKeyType(keygen.Ed25519), keygen.WithWrite())
	is.NoErr(err)

	attackerKeyPath := dp + "/attacker_key"
	attackerPair, err := keygen.New(attackerKeyPath, keygen.WithKeyType(keygen.Ed25519), keygen.WithWrite())
	is.NoErr(err)

	// Parse public keys
	adminPubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(adminPair.AuthorizedKey()))
	is.NoErr(err)

	attackerPubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(attackerPair.AuthorizedKey()))
	is.NoErr(err)

	// Create admin user
	adminUser, err := be.CreateUser(ctx, "testadmin", domain.UserOptions{
		Admin:      true,
		PublicKeys: []gossh.PublicKey{adminPubKey},
	})
	is.NoErr(err)
	is.True(adminUser != nil)

	// Create attacker (non-admin) user
	attackerUser, err := be.CreateUser(ctx, "testattacker", domain.UserOptions{
		Admin:      false,
		PublicKeys: []gossh.PublicKey{attackerPubKey},
	})
	is.NoErr(err)
	is.True(attackerUser != nil)
	is.True(!attackerUser.Admin) // Verify attacker is NOT admin

	// Test: Verify that looking up user by key gives correct user
	t.Run("user_lookup_by_key", func(t *testing.T) {
		is := is.New(t)

		// Looking up admin key should return admin user
		user, err := be.UserByPublicKey(ctx, adminPubKey)
		is.NoErr(err)
		is.Equal(user.Username, "testadmin")
		is.True(user.Admin)

		// Looking up attacker key should return attacker user
		user, err = be.UserByPublicKey(ctx, attackerPubKey)
		is.NoErr(err)
		is.Equal(user.Username, "testattacker")
		is.True(!user.Admin)
	})

	// Test: Simulate the authentication bypass vulnerability
	t.Run("authentication_bypass_simulation", func(t *testing.T) {
		is := is.New(t)

		// Create a mock context
		mockCtx := &mockSSHContext{
			Context:     ctx,
			values:      make(map[any]any),
			permissions: &ssh.Permissions{Permissions: &gossh.Permissions{Extensions: make(map[string]string)}},
		}

		// ATTACK SIMULATION:
		mockCtx.SetValue(domain.UserContextKey(), adminUser)
		mockCtx.permissions.Extensions["pubkey-fp"] = gossh.FingerprintSHA256(adminPubKey)

		mockCtx.permissions.Extensions["pubkey-fp"] = gossh.FingerprintSHA256(attackerPubKey)

		authenticatedUser, err := be.UserByPublicKey(mockCtx, attackerPubKey)
		is.NoErr(err)

		// EXPECTED: User should be "attacker", NOT "admin"
		is.Equal(authenticatedUser.Username, "testattacker")
		is.True(!authenticatedUser.Admin)

		contextUser := domain.UserFromContext(mockCtx)
		if contextUser != nil && contextUser.Username == "testadmin" {
			t.Logf("WARNING: Context still contains admin user! This indicates the vulnerability exists.")
			t.Logf("The authenticated key is attacker's, but context has admin user.")
		}
	})
}

// mockSSHContext implements ssh.Context for testing
type mockSSHContext struct {
	context.Context
	values      map[any]any
	permissions *ssh.Permissions
}

func (m *mockSSHContext) SetValue(key, value any) {
	m.values[key] = value
}

func (m *mockSSHContext) Value(key any) any {
	if v, ok := m.values[key]; ok {
		return v
	}
	return m.Context.Value(key)
}

func (m *mockSSHContext) Permissions() *ssh.Permissions {
	return m.permissions
}

func (m *mockSSHContext) User() string          { return "" }
func (m *mockSSHContext) RemoteAddr() net.Addr  { return &net.TCPAddr{} }
func (m *mockSSHContext) LocalAddr() net.Addr   { return &net.TCPAddr{} }
func (m *mockSSHContext) ServerVersion() string { return "" }
func (m *mockSSHContext) ClientVersion() string { return "" }
func (m *mockSSHContext) SessionID() string     { return "" }
func (m *mockSSHContext) Lock()                 {}
func (m *mockSSHContext) Unlock()               {}
