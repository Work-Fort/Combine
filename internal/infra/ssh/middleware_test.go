package ssh

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"charm.land/log/v2"
	"github.com/charmbracelet/keygen"
	"github.com/charmbracelet/ssh"
	"github.com/matryer/is"
	gossh "golang.org/x/crypto/ssh"

	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/domain"
	infra "github.com/Work-Fort/Combine/internal/infra"
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

	// Create identities
	adminIdentity, err := store.UpsertIdentity(ctx, "uuid-admin", "testadmin", "Test Admin", "user")
	is.NoErr(err)
	is.NoErr(store.SetIdentityAdmin(ctx, adminIdentity.ID, true))
	is.NoErr(store.AddIdentityPublicKey(ctx, adminIdentity.ID, adminPubKey))

	attackerIdentity, err := store.UpsertIdentity(ctx, "uuid-attacker", "testattacker", "Test Attacker", "user")
	is.NoErr(err)
	is.NoErr(store.AddIdentityPublicKey(ctx, attackerIdentity.ID, attackerPubKey))

	// Test: Verify that looking up identity by key gives correct identity
	t.Run("identity_lookup_by_key", func(t *testing.T) {
		is := is.New(t)

		// Looking up admin key should return admin identity
		identity, err := be.IdentityByPublicKey(ctx, adminPubKey)
		is.NoErr(err)
		is.Equal(identity.Username, "testadmin")

		// Looking up attacker key should return attacker identity
		identity, err = be.IdentityByPublicKey(ctx, attackerPubKey)
		is.NoErr(err)
		is.Equal(identity.Username, "testattacker")
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

		// ATTACK SIMULATION: pre-set admin identity but present attacker key
		mockCtx.SetValue(domain.IdentityContextKey(), adminIdentity)
		mockCtx.permissions.Extensions["pubkey-fp"] = gossh.FingerprintSHA256(adminPubKey)

		mockCtx.permissions.Extensions["pubkey-fp"] = gossh.FingerprintSHA256(attackerPubKey)

		authenticatedIdentity, err := be.IdentityByPublicKey(mockCtx, attackerPubKey)
		is.NoErr(err)

		// EXPECTED: Identity should be "attacker", NOT "admin"
		is.Equal(authenticatedIdentity.Username, "testattacker")
		is.True(!authenticatedIdentity.IsAdmin)

		contextIdentity := domain.IdentityFromContext(mockCtx)
		if contextIdentity != nil && contextIdentity.Username == "testadmin" {
			t.Logf("WARNING: Context still contains admin identity! This indicates the vulnerability exists.")
			t.Logf("The authenticated key is attacker's, but context has admin identity.")
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
