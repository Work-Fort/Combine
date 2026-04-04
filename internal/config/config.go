package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// EnvPrefix is the environment variable prefix for Combine configuration.
const EnvPrefix = "COMBINE"

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

	// DB
	viper.SetDefault("db.driver", "sqlite")
	viper.SetDefault("db.data_source", "")

	// Stats
	viper.SetDefault("stats.enabled", true)
	viper.SetDefault("stats.listen_addr", "localhost:23233")

	// Log
	viper.SetDefault("log.format", "text")
	viper.SetDefault("log.time_format", "2006-01-02 15:04:05")
	viper.SetDefault("log.path", "")

	// LFS
	viper.SetDefault("lfs.enabled", true)
	viper.SetDefault("lfs.ssh_enabled", false)

	// Jobs
	viper.SetDefault("jobs.mirror_pull", "@every 10m")

	// Config file search paths
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(GlobalPaths.ConfigDir)
	viper.AddConfigPath(GlobalPaths.StateDir)
	viper.AddConfigPath(".")

	// Environment variable binding
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
		// Convert flag name from kebab-case to dot notation used by viper
		key := strings.ReplaceAll(f.Name, "-", ".")
		_ = viper.BindPFlag(key, f)
	})
}
