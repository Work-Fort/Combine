package cmd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"charm.land/log/v2"
	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/config"
	"github.com/Work-Fort/Combine/internal/domain"
	infra "github.com/Work-Fort/Combine/internal/infra"
	"github.com/Work-Fort/Combine/internal/infra/cron"
	"github.com/Work-Fort/Combine/internal/infra/hooks"
	"github.com/Work-Fort/Combine/internal/infra/httpapi"
	"github.com/Work-Fort/Combine/internal/infra/jobs"
	sshsrv "github.com/Work-Fort/Combine/internal/infra/ssh"
	"github.com/Work-Fort/Combine/internal/infra/stats"
	"github.com/charmbracelet/ssh"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// NewDaemonCmd creates the daemon command.
func NewDaemonCmd() *cobra.Command {
	var syncHooksFlag bool

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Combine daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(cmd.Context(), syncHooksFlag)
		},
	}
	cmd.Flags().BoolVar(&syncHooksFlag, "sync-hooks", false, "Synchronize hooks for all repositories before starting")
	return cmd
}

// NewServeCmd creates the serve command (alias for daemon, backward compat).
func NewServeCmd() *cobra.Command {
	var syncHooksFlag bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the server (alias for daemon)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(cmd.Context(), syncHooksFlag)
		},
	}
	cmd.Flags().BoolVar(&syncHooksFlag, "sync-hooks", false, "Synchronize hooks for all repositories before starting")
	return cmd
}

func runDaemon(ctx context.Context, syncHooksFlag bool) error {
	cfg := config.FromContext(ctx)
	logger := log.FromContext(ctx)
	if logger == nil {
		logger = log.Default()
	}

	// Ensure data directories exist
	for _, dir := range []string{
		cfg.DataPath,
		filepath.Join(cfg.DataPath, "repos"),
		filepath.Join(cfg.DataPath, "log"),
		filepath.Join(cfg.DataPath, "hooks"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Generate update hook example if hooks dir is new
	hookSample := filepath.Join(cfg.DataPath, "hooks", "update.sample")
	if _, err := os.Stat(hookSample); os.IsNotExist(err) {
		os.WriteFile(hookSample, []byte(updateHookExample), 0o744) //nolint: errcheck,gosec
	}

	// Open store
	store, err := infra.Open(cfg.DB.DataSource)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	ctx = domain.WithStoreContext(ctx, store)

	// Create backend
	beCfg := backend.BackendConfig{
		RepoDir:            filepath.Join(cfg.DataPath, "repos"),
		DataDir:            cfg.DataPath,
		AdminKeys:          cfg.AdminKeys(),
		SSHClientKeyPath:   cfg.SSH.ClientKeyPath,
		SSHKnownHostsPath: filepath.Join(cfg.DataPath, "ssh", "known_hosts"),
	}
	be := backend.New(ctx, store, beCfg, logger.WithPrefix("backend"))
	ctx = backend.WithContext(ctx, be)

	// Sync hooks if requested
	if syncHooksFlag {
		if err := initializeHooks(ctx, cfg, be); err != nil {
			return fmt.Errorf("initialize hooks: %w", err)
		}
	}

	// Cron jobs
	sched := cron.NewScheduler(ctx)
	for n, j := range jobs.List() {
		id, err := sched.AddFunc(j.Runner.Spec(ctx), j.Runner.Func(ctx))
		if err != nil {
			logger.Warn("error adding cron job", "job", n, "err", err)
		}
		j.ID = id
	}

	// Create SSH server
	sshServer, err := sshsrv.NewSSHServer(ctx)
	if err != nil {
		return fmt.Errorf("create ssh server: %w", err)
	}

	// Passport auth (optional — only enabled when passport-url is configured)
	var passport *web.PassportAuth
	if cfg.PassportURL != "" {
		var passportErr error
		passport, passportErr = web.NewPassportAuth(ctx, cfg.PassportURL, store)
		if passportErr != nil {
			return fmt.Errorf("init passport auth: %w", passportErr)
		}
		defer passport.Close()
	}

	// Create HTTP server
	httpServer, err := web.NewHTTPServer(ctx, passport)
	if err != nil {
		return fmt.Errorf("create http server: %w", err)
	}

	// Create Stats server
	statsServer, err := stats.NewStatsServer(ctx)
	if err != nil {
		return fmt.Errorf("create stats server: %w", err)
	}

	// TLS cert reloader
	var certReloader *CertReloader
	if cfg.HTTP.TLSKeyPath != "" && cfg.HTTP.TLSCertPath != "" {
		certReloader, err = NewCertReloader(cfg.HTTP.TLSCertPath, cfg.HTTP.TLSKeyPath, logger)
		if err != nil {
			return fmt.Errorf("create cert reloader: %w", err)
		}
		httpServer.SetTLSConfig(&tls.Config{
			GetCertificate: certReloader.GetCertificateFunc(),
		})
	}

	// Test run stop endpoint
	if cfg.TestRun {
		h := httpServer.Server.Handler
		done := make(chan struct{})
		var doneOnce sync.Once
		httpServer.Server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/__stop" && r.Method == http.MethodHead {
				doneOnce.Do(func() { close(done) })
				return
			}
			h.ServeHTTP(w, r)
		})
		// Monitor the done channel to initiate shutdown
		go func() {
			<-done
			syscall.Kill(syscall.Getpid(), syscall.SIGTERM) //nolint: errcheck
		}()
	}

	// Start servers
	lch := make(chan error, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		errg, _ := errgroup.WithContext(ctx)

		if cfg.SSH.Enabled {
			errg.Go(func() error {
				logger.Print("Starting SSH server", "addr", cfg.SSH.ListenAddr)
				if err := sshServer.ListenAndServe(); !errors.Is(err, ssh.ErrServerClosed) {
					return err
				}
				return nil
			})
		}

		if cfg.HTTP.Enabled {
			errg.Go(func() error {
				logger.Print("Starting HTTP server", "addr", cfg.HTTP.ListenAddr)
				if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
					return err
				}
				return nil
			})
		}

		if cfg.Stats.Enabled {
			errg.Go(func() error {
				logger.Print("Starting Stats server", "addr", cfg.Stats.ListenAddr)
				if err := statsServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
					return err
				}
				return nil
			})
		}

		errg.Go(func() error {
			sched.Start()
			return nil
		})

		lch <- errg.Wait()
	}()

	for {
		select {
		case err := <-lch:
			if err != nil {
				return fmt.Errorf("server error: %w", err)
			}
		case sig := <-sigCh:
			if sig == syscall.SIGHUP {
				logger.Info("received SIGHUP, reloading TLS certificates if enabled")
				if certReloader != nil {
					if err := certReloader.Reload(); err != nil {
						logger.Error("failed to reload TLS certificates", "err", err)
					}
				}
				continue
			}
			logger.Info("received signal, shutting down", "signal", sig)
		}
		break
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var shutdownGroup errgroup.Group
	shutdownGroup.Go(func() error { return httpServer.Shutdown(shutdownCtx) })
	shutdownGroup.Go(func() error { return sshServer.Shutdown(shutdownCtx) })
	shutdownGroup.Go(func() error { return statsServer.Shutdown(shutdownCtx) })
	shutdownGroup.Go(func() error {
		for _, j := range jobs.List() {
			sched.Remove(j.ID)
		}
		sched.Stop()
		return nil
	})

	return shutdownGroup.Wait()
}

func initializeHooks(ctx context.Context, cfg *config.Config, be *backend.Backend) error {
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

const updateHookExample = `#!/bin/sh
#
# An example hook script to echo information about the push
# and send it to the client.
#
# To enable this hook, rename this file to "update" and make it executable.

refname="$1"
oldrev="$2"
newrev="$3"

# Safety check
if [ -z "$GIT_DIR" ]; then
        echo "Don't run this script from the command line." >&2
        echo " (if you want, you could supply GIT_DIR then run" >&2
        echo "  $0 <ref> <oldrev> <newrev>)" >&2
        exit 1
fi

if [ -z "$refname" -o -z "$oldrev" -o -z "$newrev" ]; then
        echo "usage: $0 <ref> <oldrev> <newrev>" >&2
        exit 1
fi

# Check types
# if $newrev is 0000...0000, it's a commit to delete a ref.
zero=$(git hash-object --stdin </dev/null | tr '[0-9a-f]' '0')
if [ "$newrev" = "$zero" ]; then
        newrev_type=delete
else
        newrev_type=$(git cat-file -t $newrev)
fi

echo "Hi from Combine update hook!"
echo
echo "Repository: $COMBINE_REPO_NAME"
echo "RefName: $refname"
echo "Change Type: $newrev_type"
echo "Old SHA1: $oldrev"
echo "New SHA1: $newrev"

exit 0
`
