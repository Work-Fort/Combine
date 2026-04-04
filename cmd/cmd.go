package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"charm.land/log/v2"
	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/domain"
	infra "github.com/Work-Fort/Combine/internal/infra"
	"github.com/Work-Fort/Combine/internal/infra/hooks"
	"github.com/Work-Fort/Combine/pkg/config"
	"github.com/spf13/cobra"
)

// InitBackendContext initializes the backend context.
func InitBackendContext(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	cfg := config.FromContext(ctx)
	logger := log.FromContext(ctx)
	if _, err := os.Stat(cfg.DataPath); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(cfg.DataPath, os.ModePerm); err != nil {
			return fmt.Errorf("create data directory: %w", err)
		}
	}

	store, err := infra.Open(cfg.DB.DataSource)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	ctx = domain.WithStoreContext(ctx, store)

	beCfg := backend.BackendConfig{
		RepoDir:            filepath.Join(cfg.DataPath, "repos"),
		DataDir:            cfg.DataPath,
		AdminKeys:          cfg.AdminKeys(),
		SSHClientKeyPath:   cfg.SSH.ClientKeyPath,
		SSHKnownHostsPath: filepath.Join(cfg.DataPath, "ssh", "known_hosts"),
	}
	be := backend.New(ctx, store, beCfg, logger.WithPrefix("backend"))
	ctx = backend.WithContext(ctx, be)

	cmd.SetContext(ctx)

	return nil
}

// CloseStoreContext closes the store context.
func CloseStoreContext(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	store := domain.StoreFromContext(ctx)
	if store != nil {
		if err := store.Close(); err != nil {
			return fmt.Errorf("close store: %w", err)
		}
	}

	return nil
}

// InitializeHooks initializes the hooks.
func InitializeHooks(ctx context.Context, cfg *config.Config, be *backend.Backend) error {
	repos, err := be.Repositories(ctx)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		if err := hooks.GenerateHooks(ctx, cfg, repo.Name); err != nil {
			return err
		}
	}

	return nil
}
