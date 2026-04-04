package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Work-Fort/Combine/internal/infra/sshutils"
	"github.com/charmbracelet/keygen"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"
)

// EnvPrefix is the environment variable prefix for Combine configuration.
const EnvPrefix = "COMBINE"

var binPath = "combine"

func init() {
	if ex, err := os.Executable(); err == nil {
		binPath = filepath.ToSlash(ex)
	}
}

// Paths holds XDG-compliant configuration and state directory paths.
type Paths struct {
	ConfigDir string
	StateDir  string
}

// GlobalPaths is the package-level Paths instance, initialized at startup.
var GlobalPaths Paths

func init() {
	GlobalPaths = GetPaths()
}

// GetPaths returns XDG-compliant paths for Combine, respecting
// XDG_CONFIG_HOME and XDG_STATE_HOME with sensible fallbacks.
func GetPaths() Paths {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}

	return Paths{
		ConfigDir: filepath.Join(configHome, "combine"),
		StateDir:  filepath.Join(stateHome, "combine"),
	}
}

// InitDirs ensures the config and state directories exist.
func InitDirs() error {
	if err := os.MkdirAll(GlobalPaths.ConfigDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(GlobalPaths.StateDir, 0o755)
}

// InitViper sets all default values, config search paths, and env binding.
func InitViper() {
	// Server
	viper.SetDefault("name", "Combine")
	viper.SetDefault("data_path", "data")

	// SSH
	viper.SetDefault("ssh.enabled", true)
	viper.SetDefault("ssh.listen_addr", ":23231")
	viper.SetDefault("ssh.public_url", "ssh://localhost:23231")
	viper.SetDefault("ssh.key_path", filepath.Join("ssh", "combine_host_ed25519"))
	viper.SetDefault("ssh.client_key_path", filepath.Join("ssh", "combine_client_ed25519"))
	viper.SetDefault("ssh.max_timeout", 0)
	viper.SetDefault("ssh.idle_timeout", 600) // 10 minutes

	// HTTP
	viper.SetDefault("http.enabled", true)
	viper.SetDefault("http.listen_addr", ":23232")
	viper.SetDefault("http.public_url", "http://localhost:23232")
	viper.SetDefault("http.tls_key_path", "")
	viper.SetDefault("http.tls_cert_path", "")

	// CORS
	viper.SetDefault("http.cors.allowed_headers", []string{
		"Accept", "Accept-Language", "Content-Language", "Content-Type",
		"Origin", "X-Requested-With", "User-Agent", "Authorization",
		"Access-Control-Request-Method", "Access-Control-Allow-Origin",
	})
	viper.SetDefault("http.cors.allowed_origins", []string{"http://localhost:23232"})
	viper.SetDefault("http.cors.allowed_methods", []string{"GET", "HEAD", "POST", "PUT", "OPTIONS"})

	// DB
	viper.SetDefault("db.driver", "sqlite")
	viper.SetDefault("db.data_source", "")

	// Stats
	viper.SetDefault("stats.enabled", true)
	viper.SetDefault("stats.listen_addr", "localhost:23233")

	// Log
	viper.SetDefault("log.format", "text")
	viper.SetDefault("log.time_format", time.DateTime)
	viper.SetDefault("log.path", "")

	// LFS
	viper.SetDefault("lfs.enabled", true)
	viper.SetDefault("lfs.ssh_enabled", false)

	// Jobs
	viper.SetDefault("jobs.mirror_pull", "@every 10m")

	// Initial admin keys
	viper.SetDefault("initial_admin_keys", []string{})

	// Passport
	viper.SetDefault("passport-url", "")

	// Test run
	viper.SetDefault("testrun", false)

	// Config file search paths.
	// We use "combine" as the config name (not "config") to avoid
	// collisions with git's config file when hooks run from a bare repo.
	viper.SetConfigName("combine")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(GlobalPaths.ConfigDir)
	viper.AddConfigPath(GlobalPaths.StateDir)
	viper.AddConfigPath(".")

	// Environment variable binding
	// The replacer converts both "." and "-" to "_", so:
	//   COMBINE_DATA_PATH -> data_path
	//   COMBINE_SSH_LISTEN_ADDR -> ssh.listen_addr (via ssh_listen_addr)
	//   COMBINE_INITIAL_ADMIN_KEYS -> initial_admin_keys
	//   COMBINE_STATS_ENABLED -> stats.enabled (via stats_enabled)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()
}

// LoadConfig reads the config file from disk. It silently ignores
// a missing config file (first-run scenario) but returns other errors.
func LoadConfig() error {
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		return err
	}
	return nil
}

// BindFlags binds a pflag.FlagSet to viper keys so CLI flags override
// config-file and environment values.
func BindFlags(flags *pflag.FlagSet) {
	flags.VisitAll(func(f *pflag.Flag) {
		// Flag names use kebab-case; viper keys use underscores/dots.
		// Convert "ssh-listen-addr" -> "ssh.listen.addr" won't work; bind directly.
		_ = viper.BindPFlag(f.Name, f)
	})
}

// -----------------------------------------------------------------------
// Config struct — mirrors legacy config.Config so infra packages that use
// config.FromContext(ctx) keep compiling with zero changes.
// -----------------------------------------------------------------------

// SSHConfig is the configuration for the SSH server.
type SSHConfig struct {
	Enabled       bool
	ListenAddr    string
	PublicURL     string
	KeyPath       string
	ClientKeyPath string
	MaxTimeout    int
	IdleTimeout   int
}

// CORSConfig is the CORS configuration for the server.
type CORSConfig struct {
	AllowedHeaders []string
	AllowedOrigins []string
	AllowedMethods []string
}

// HTTPConfig is the HTTP configuration for the server.
type HTTPConfig struct {
	Enabled     bool
	ListenAddr  string
	TLSKeyPath  string
	TLSCertPath string
	PublicURL   string
	CORS        CORSConfig
}

// StatsConfig is the configuration for the stats server.
type StatsConfig struct {
	Enabled    bool
	ListenAddr string
}

// LogConfig is the logger configuration.
type LogConfig struct {
	Format     string
	TimeFormat string
	Path       string
}

// DBConfig is the database connection configuration.
type DBConfig struct {
	Driver     string
	DataSource string
}

// LFSConfig is the configuration for Git LFS.
type LFSConfig struct {
	Enabled    bool
	SSHEnabled bool
}

// JobsConfig is the configuration for cron jobs.
type JobsConfig struct {
	MirrorPull string
}

// Config is the configuration for Combine.
type Config struct {
	Name             string
	SSH              SSHConfig
	HTTP             HTTPConfig
	Stats            StatsConfig
	Log              LogConfig
	DB               DBConfig
	LFS              LFSConfig
	Jobs             JobsConfig
	InitialAdminKeys []string
	DataPath         string
	PassportURL      string
	TestRun          bool
}

// -----------------------------------------------------------------------
// Viper → Config
// -----------------------------------------------------------------------

// FromViper builds a Config from the current viper state, resolving
// relative paths against DataPath and validating the result.
func FromViper() (*Config, error) {
	dataPath := viper.GetString("data_path")
	if dataPath == "" {
		dataPath = "data"
	}
	if !filepath.IsAbs(dataPath) {
		abs, err := filepath.Abs(dataPath)
		if err != nil {
			return nil, err
		}
		dataPath = abs
	}

	sshKeyPath := viper.GetString("ssh.key_path")
	if sshKeyPath != "" && !filepath.IsAbs(sshKeyPath) {
		sshKeyPath = filepath.Join(dataPath, sshKeyPath)
	}

	sshClientKeyPath := viper.GetString("ssh.client_key_path")
	if sshClientKeyPath != "" && !filepath.IsAbs(sshClientKeyPath) {
		sshClientKeyPath = filepath.Join(dataPath, sshClientKeyPath)
	}

	tlsKeyPath := viper.GetString("http.tls_key_path")
	if tlsKeyPath != "" && !filepath.IsAbs(tlsKeyPath) {
		tlsKeyPath = filepath.Join(dataPath, tlsKeyPath)
	}

	tlsCertPath := viper.GetString("http.tls_cert_path")
	if tlsCertPath != "" && !filepath.IsAbs(tlsCertPath) {
		tlsCertPath = filepath.Join(dataPath, tlsCertPath)
	}

	dbDataSource := viper.GetString("db.data_source")
	dbDriver := viper.GetString("db.driver")
	if dbDataSource == "" {
		dbDataSource = "combine.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	if strings.HasPrefix(dbDriver, "sqlite") && !filepath.IsAbs(dbDataSource) {
		dbDataSource = filepath.Join(dataPath, dbDataSource)
	}

	httpPublicURL := strings.TrimSuffix(viper.GetString("http.public_url"), "/")
	sshPublicURL := strings.TrimSuffix(viper.GetString("ssh.public_url"), "/")

	corsOrigins := viper.GetStringSlice("http.cors.allowed_origins")
	corsOrigins = append([]string{httpPublicURL}, corsOrigins...)

	// Parse and validate admin keys.
	// The env var COMBINE_INITIAL_ADMIN_KEYS is a newline-separated list of
	// authorized_keys strings. Viper's GetStringSlice would split on spaces
	// which breaks SSH keys, so we use GetString and split manually.
	rawKeys := getAdminKeysFromViper()
	pks := make([]string, 0)
	for _, key := range ParseAdminKeys(rawKeys) {
		ak := sshutils.MarshalAuthorizedKey(key)
		pks = append(pks, ak)
	}

	cfg := &Config{
		Name:        viper.GetString("name"),
		DataPath:    dataPath,
		PassportURL: viper.GetString("passport-url"),
		TestRun:     viper.GetBool("testrun"),
		SSH: SSHConfig{
			Enabled:       viper.GetBool("ssh.enabled"),
			ListenAddr:    viper.GetString("ssh.listen_addr"),
			PublicURL:     sshPublicURL,
			KeyPath:       sshKeyPath,
			ClientKeyPath: sshClientKeyPath,
			MaxTimeout:    viper.GetInt("ssh.max_timeout"),
			IdleTimeout:   viper.GetInt("ssh.idle_timeout"),
		},
		HTTP: HTTPConfig{
			Enabled:     viper.GetBool("http.enabled"),
			ListenAddr:  viper.GetString("http.listen_addr"),
			TLSKeyPath:  tlsKeyPath,
			TLSCertPath: tlsCertPath,
			PublicURL:   httpPublicURL,
			CORS: CORSConfig{
				AllowedHeaders: viper.GetStringSlice("http.cors.allowed_headers"),
				AllowedOrigins: corsOrigins,
				AllowedMethods: viper.GetStringSlice("http.cors.allowed_methods"),
			},
		},
		Stats: StatsConfig{
			Enabled:    viper.GetBool("stats.enabled"),
			ListenAddr: viper.GetString("stats.listen_addr"),
		},
		Log: LogConfig{
			Format:     viper.GetString("log.format"),
			TimeFormat: viper.GetString("log.time_format"),
			Path:       viper.GetString("log.path"),
		},
		DB: DBConfig{
			Driver:     dbDriver,
			DataSource: dbDataSource,
		},
		LFS: LFSConfig{
			Enabled:    viper.GetBool("lfs.enabled"),
			SSHEnabled: viper.GetBool("lfs.ssh_enabled"),
		},
		Jobs: JobsConfig{
			MirrorPull: viper.GetString("jobs.mirror_pull"),
		},
		InitialAdminKeys: pks,
	}

	return cfg, nil
}

// getAdminKeysFromViper reads the initial admin keys from viper, handling
// both YAML (string slice) and env var (newline-separated string) sources.
func getAdminKeysFromViper() []string {
	// Try as string first (handles env vars correctly without space-splitting)
	raw := viper.GetString("initial_admin_keys")
	if raw != "" {
		var keys []string
		for _, k := range strings.Split(raw, "\n") {
			k = strings.TrimSpace(k)
			if k != "" {
				keys = append(keys, k)
			}
		}
		return keys
	}

	// Fall back to string slice (from YAML config files)
	return viper.GetStringSlice("initial_admin_keys")
}

// DefaultConfig returns a Config with default values, suitable for tests.
func DefaultConfig() *Config {
	dp := os.Getenv("COMBINE_DATA_PATH")
	if dp == "" {
		dp = "data"
	}
	if !filepath.IsAbs(dp) {
		abs, _ := filepath.Abs(dp)
		dp = abs
	}

	sshKeyPath := filepath.Join(dp, "ssh", "combine_host_ed25519")
	sshClientKeyPath := filepath.Join(dp, "ssh", "combine_client_ed25519")

	return &Config{
		Name:     "Combine",
		DataPath: dp,
		SSH: SSHConfig{
			Enabled:       true,
			ListenAddr:    ":23231",
			PublicURL:     "ssh://localhost:23231",
			KeyPath:       sshKeyPath,
			ClientKeyPath: sshClientKeyPath,
			MaxTimeout:    0,
			IdleTimeout:   10 * 60,
		},
		HTTP: HTTPConfig{
			Enabled:    true,
			ListenAddr: ":23232",
			PublicURL:  "http://localhost:23232",
			CORS: CORSConfig{
				AllowedHeaders: []string{"Accept", "Accept-Language", "Content-Language", "Content-Type", "Origin", "X-Requested-With", "User-Agent", "Authorization", "Access-Control-Request-Method", "Access-Control-Allow-Origin"},
				AllowedMethods: []string{"GET", "HEAD", "POST", "PUT", "OPTIONS"},
				AllowedOrigins: []string{"http://localhost:23232"},
			},
		},
		Stats: StatsConfig{
			Enabled:    true,
			ListenAddr: "localhost:23233",
		},
		Log: LogConfig{
			Format:     "text",
			TimeFormat: time.DateTime,
		},
		DB: DBConfig{
			Driver:     "sqlite",
			DataSource: filepath.Join(dp, "combine.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"),
		},
		LFS: LFSConfig{
			Enabled:    true,
			SSHEnabled: false,
		},
		Jobs: JobsConfig{
			MirrorPull: "@every 10m",
		},
	}
}

// -----------------------------------------------------------------------
// Context plumbing
// -----------------------------------------------------------------------

// ContextKey is the context key for the config.
var ContextKey = struct{ string }{"config"}

// WithContext returns a new context with the configuration attached.
func WithContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, ContextKey, cfg)
}

// FromContext returns the configuration from the context.
// It accepts any type with a Value method (context.Context, ssh.Context, etc.).
func FromContext(ctx interface{ Value(any) any }) *Config {
	if c, ok := ctx.Value(ContextKey).(*Config); ok {
		return c
	}
	return nil
}

// -----------------------------------------------------------------------
// Environ — produces the env list passed to git hooks.
// -----------------------------------------------------------------------

// Environ returns the config as a list of environment variables.
func (c *Config) Environ() []string {
	envs := []string{
		fmt.Sprintf("COMBINE_BIN_PATH=%s", binPath),
	}
	if c == nil {
		return envs
	}

	envs = append(envs, []string{
		fmt.Sprintf("COMBINE_DATA_PATH=%s", c.DataPath),
		fmt.Sprintf("COMBINE_NAME=%s", c.Name),
		fmt.Sprintf("COMBINE_INITIAL_ADMIN_KEYS=%s", strings.Join(c.InitialAdminKeys, "\n")),
		fmt.Sprintf("COMBINE_SSH_ENABLED=%t", c.SSH.Enabled),
		fmt.Sprintf("COMBINE_SSH_LISTEN_ADDR=%s", c.SSH.ListenAddr),
		fmt.Sprintf("COMBINE_SSH_PUBLIC_URL=%s", c.SSH.PublicURL),
		fmt.Sprintf("COMBINE_SSH_KEY_PATH=%s", c.SSH.KeyPath),
		fmt.Sprintf("COMBINE_SSH_CLIENT_KEY_PATH=%s", c.SSH.ClientKeyPath),
		fmt.Sprintf("COMBINE_SSH_MAX_TIMEOUT=%d", c.SSH.MaxTimeout),
		fmt.Sprintf("COMBINE_SSH_IDLE_TIMEOUT=%d", c.SSH.IdleTimeout),
		fmt.Sprintf("COMBINE_HTTP_ENABLED=%t", c.HTTP.Enabled),
		fmt.Sprintf("COMBINE_HTTP_LISTEN_ADDR=%s", c.HTTP.ListenAddr),
		fmt.Sprintf("COMBINE_HTTP_TLS_KEY_PATH=%s", c.HTTP.TLSKeyPath),
		fmt.Sprintf("COMBINE_HTTP_TLS_CERT_PATH=%s", c.HTTP.TLSCertPath),
		fmt.Sprintf("COMBINE_HTTP_PUBLIC_URL=%s", c.HTTP.PublicURL),
		fmt.Sprintf("COMBINE_HTTP_CORS_ALLOWED_HEADERS=%s", strings.Join(c.HTTP.CORS.AllowedHeaders, ",")),
		fmt.Sprintf("COMBINE_HTTP_CORS_ALLOWED_ORIGINS=%s", strings.Join(c.HTTP.CORS.AllowedOrigins, ",")),
		fmt.Sprintf("COMBINE_HTTP_CORS_ALLOWED_METHODS=%s", strings.Join(c.HTTP.CORS.AllowedMethods, ",")),
		fmt.Sprintf("COMBINE_STATS_ENABLED=%t", c.Stats.Enabled),
		fmt.Sprintf("COMBINE_STATS_LISTEN_ADDR=%s", c.Stats.ListenAddr),
		fmt.Sprintf("COMBINE_LOG_FORMAT=%s", c.Log.Format),
		fmt.Sprintf("COMBINE_LOG_TIME_FORMAT=%s", c.Log.TimeFormat),
		fmt.Sprintf("COMBINE_DB_DRIVER=%s", c.DB.Driver),
		fmt.Sprintf("COMBINE_DB_DATA_SOURCE=%s", c.DB.DataSource),
		fmt.Sprintf("COMBINE_LFS_ENABLED=%t", c.LFS.Enabled),
		fmt.Sprintf("COMBINE_LFS_SSH_ENABLED=%t", c.LFS.SSHEnabled),
		fmt.Sprintf("COMBINE_JOBS_MIRROR_PULL=%s", c.Jobs.MirrorPull),
	}...)

	return envs
}

// -----------------------------------------------------------------------
// SSH key helpers
// -----------------------------------------------------------------------

var (
	// ErrNilConfig is returned when a nil config is passed to a function.
	ErrNilConfig = fmt.Errorf("nil config")

	// ErrEmptySSHKeyPath is returned when the SSH key path is empty.
	ErrEmptySSHKeyPath = fmt.Errorf("empty SSH key path")
)

// KeyPair returns the server's SSH key pair.
func KeyPair(cfg *Config) (*keygen.SSHKeyPair, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}
	if cfg.SSH.KeyPath == "" {
		return nil, ErrEmptySSHKeyPath
	}
	return keygen.New(cfg.SSH.KeyPath, keygen.WithKeyType(keygen.Ed25519))
}

// AdminKeys returns the parsed SSH public keys from the config.
func (c *Config) AdminKeys() []ssh.PublicKey {
	return ParseAdminKeys(c.InitialAdminKeys)
}

// ParseAdminKeys parses authorized keys from either file paths or
// authorized_keys formatted strings.
func ParseAdminKeys(aks []string) []ssh.PublicKey {
	exist := make(map[string]struct{})
	pks := make([]ssh.PublicKey, 0)
	for _, key := range aks {
		if bts, err := os.ReadFile(key); err == nil {
			key = strings.TrimSpace(string(bts))
		}
		if pk, _, err := sshutils.ParseAuthorizedKey(key); err == nil {
			if _, ok := exist[key]; !ok {
				pks = append(pks, pk)
				exist[key] = struct{}{}
			}
		}
	}
	return pks
}

// IsDebug returns true if the server is running in debug mode.
func IsDebug() bool {
	debug := os.Getenv("COMBINE_DEBUG")
	return debug == "1" || debug == "true"
}

// IsVerbose returns true if the server is running in verbose mode.
func IsVerbose() bool {
	verbose := os.Getenv("COMBINE_VERBOSE")
	return IsDebug() && (verbose == "1" || verbose == "true")
}
