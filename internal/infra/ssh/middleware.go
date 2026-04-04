package ssh

import (
	"fmt"
	"strconv"
	"time"

	"charm.land/log/v2"
	"charm.land/wish/v2"
	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/pkg/config"
	"github.com/Work-Fort/Combine/internal/infra/ssh/cmd"
	"github.com/Work-Fort/Combine/internal/infra/sshutils"
	"github.com/charmbracelet/ssh"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
)

// ErrPermissionDenied is returned when a user is not allowed connect.
var ErrPermissionDenied = fmt.Errorf("permission denied")

// AuthenticationMiddleware handles authentication.
func AuthenticationMiddleware(sh ssh.Handler) ssh.Handler {
	return func(s ssh.Session) {
		ctx := s.Context()
		be := backend.FromContext(ctx)

		var pkFp string
		perms := s.Permissions().Permissions
		pk := s.PublicKey()
		if pk != nil {
			if perms == nil {
				wish.Fatalln(s, ErrPermissionDenied)
				return
			}

			pkFp = gossh.FingerprintSHA256(pk)
		}

		fp := perms.Extensions["pubkey-fp"]
		if fp != "" && fp != pkFp {
			wish.Fatalln(s, ErrPermissionDenied)
			return
		}

		ac := be.AllowKeyless(ctx)
		publicKeyCounter.WithLabelValues(strconv.FormatBool(ac || pk != nil)).Inc()
		if !ac && pk == nil {
			wish.Fatalln(s, ErrPermissionDenied)
			return
		}

		// Set the auth'd user, or anon, in the context
		var user *domain.User
		if pk != nil {
			user, _ = be.UserByPublicKey(ctx, pk)
		}
		ctx.SetValue(domain.UserContextKey(), user)

		sh(s)
	}
}

// ContextMiddleware adds the config, backend, store, and logger to the session context.
func ContextMiddleware(cfg *config.Config, datastore domain.Store, be *backend.Backend, logger *log.Logger) func(ssh.Handler) ssh.Handler {
	return func(sh ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			ctx := s.Context()
			ctx.SetValue(sshutils.ContextKeySession, s)
			ctx.SetValue(config.ContextKey, cfg)
			ctx.SetValue(domain.StoreContextKey(), datastore)
			ctx.SetValue(backend.ContextKey, be)
			ctx.SetValue(log.ContextKey, logger.WithPrefix("ssh"))
			sh(s)
		}
	}
}

var cliCommandCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "combine",
	Subsystem: "cli",
	Name:      "commands_total",
	Help:      "Total times each command was called",
}, []string{"command"})

// CommandMiddleware handles git commands and CLI commands.
func CommandMiddleware(sh ssh.Handler) ssh.Handler {
	return func(s ssh.Session) {
		_, _, ptyReq := s.Pty()
		if ptyReq {
			fmt.Fprintln(s, "Interactive sessions are not supported. Use SSH commands instead.")
			fmt.Fprintln(s, "Example: ssh <host> -p <port> help")
			return
		}

		ctx := s.Context()
		cfg := config.FromContext(ctx)

		args := s.Command()
		cliCommandCounter.WithLabelValues(cmd.CommandName(args)).Inc()
		rootCmd := &cobra.Command{
			Short:        "Combine is a self-hostable Git forge for the WorkFort platform.",
			SilenceUsage: true,
		}
		rootCmd.CompletionOptions.DisableDefaultCmd = true

		rootCmd.SetUsageTemplate(cmd.UsageTemplate)
		rootCmd.SetUsageFunc(cmd.UsageFunc)
		rootCmd.AddCommand(
			cmd.GitUploadPackCommand(),
			cmd.GitUploadArchiveCommand(),
			cmd.GitReceivePackCommand(),
		)

		if cfg.LFS.Enabled {
			rootCmd.AddCommand(
				cmd.GitLFSAuthenticateCommand(),
			)

			if cfg.LFS.SSHEnabled {
				rootCmd.AddCommand(
					cmd.GitLFSTransfer(),
				)
			}
		}

		rootCmd.SetArgs(args)
		if len(args) == 0 {
			rootCmd.SetArgs([]string{"--help"})
		}
		rootCmd.SetIn(s)
		rootCmd.SetOut(s)
		rootCmd.SetErr(s.Stderr())
		rootCmd.SetContext(ctx)

		if err := rootCmd.ExecuteContext(ctx); err != nil {
			s.Exit(1) //nolint: errcheck
			return
		}
	}
}

// LoggingMiddleware logs the ssh connection and command.
func LoggingMiddleware(sh ssh.Handler) ssh.Handler {
	return func(s ssh.Session) {
		ctx := s.Context()
		logger := log.FromContext(ctx).WithPrefix("ssh")
		ct := time.Now()
		hpk := sshutils.MarshalAuthorizedKey(s.PublicKey())
		ptyReq, _, isPty := s.Pty()
		addr := s.RemoteAddr().String()
		user := domain.UserFromContext(ctx)
		logArgs := []interface{}{
			"addr",
			addr,
			"cmd",
			s.Command(),
		}

		if user != nil {
			logArgs = append([]interface{}{
				"username",
				user.Username,
			}, logArgs...)
		}

		if isPty {
			logArgs = []interface{}{
				"term", ptyReq.Term,
				"width", ptyReq.Window.Width,
				"height", ptyReq.Window.Height,
			}
		}

		if config.IsVerbose() {
			logArgs = append(logArgs,
				"key", hpk,
				"envs", s.Environ(),
			)
		}

		msg := fmt.Sprintf("user %q", s.User())
		logger.Debug(msg+" connected", logArgs...)
		sh(s)
		logger.Debug(msg+" disconnected", append(logArgs, "duration", time.Since(ct))...)
	}
}
