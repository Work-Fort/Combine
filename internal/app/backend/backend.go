package backend

import (
	"context"

	"charm.land/log/v2"
	"golang.org/x/crypto/ssh"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/task"
)

// BackendConfig holds the configuration for the backend.
type BackendConfig struct {
	RepoDir           string
	DataDir           string
	AdminKeys         []ssh.PublicKey
	SSHClientKeyPath  string
	SSHKnownHostsPath string
}

// Backend is the Combine backend that handles users, repositories, and
// server settings management and operations.
type Backend struct {
	store   domain.Store
	cfg     BackendConfig
	logger  *log.Logger
	cache   *cache
	manager *task.Manager
}

// New returns a new Combine backend.
func New(ctx context.Context, store domain.Store, cfg BackendConfig, logger *log.Logger) *Backend {
	if logger == nil {
		logger = log.FromContext(ctx).WithPrefix("backend")
	}
	b := &Backend{
		store:   store,
		cfg:     cfg,
		logger:  logger,
		manager: task.NewManager(ctx),
	}

	// TODO: implement a proper caching interface
	cache := newCache(b, 1000)
	b.cache = cache

	return b
}

// Store returns the backend's store.
func (d *Backend) Store() domain.Store {
	return d.store
}

// Config returns the backend's config.
func (d *Backend) Config() BackendConfig {
	return d.cfg
}
