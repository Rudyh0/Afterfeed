// Package config reads Afterfeed's runtime configuration from the environment.
//
// Afterfeed needs no configuration file. Everything has a sensible default for
// the Docker image, and settings that users change day to day live in the
// database and are edited in the web interface.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the runtime configuration.
type Config struct {
	// ConfigDir holds the database and other small system files.
	ConfigDir string
	// DataDir holds the user's memories, messages and imports. It is the source of truth.
	DataDir string
	// CacheDir holds thumbnails, previews and other files that can be regenerated.
	CacheDir string
	// ImportDir is watched for export archives dropped in by the user.
	ImportDir string

	Host string
	Port int

	// SecureCookies is "auto", "true" or "false". In auto mode cookies are marked
	// secure when the request arrived over HTTPS or through a proxy that says so.
	SecureCookies string

	// PreviewURL is an optional document conversion service (Gotenberg) used to
	// render previews of Word, Excel and PowerPoint files.
	PreviewURL string

	LogLevel string

	// Dev enables development behaviour, such as permissive CORS for the
	// frontend dev server.
	Dev bool
}

// Load reads the configuration from environment variables.
func Load() (*Config, error) {
	c := &Config{
		ConfigDir:     env("AFTERFEED_CONFIG_DIR", "/config"),
		DataDir:       env("AFTERFEED_DATA_DIR", "/data"),
		Host:          env("AFTERFEED_HOST", "0.0.0.0"),
		SecureCookies: strings.ToLower(env("AFTERFEED_SECURE_COOKIES", "auto")),
		PreviewURL:    strings.TrimRight(env("AFTERFEED_PREVIEW_URL", ""), "/"),
		LogLevel:      strings.ToLower(env("AFTERFEED_LOG_LEVEL", "info")),
		Dev:           env("AFTERFEED_DEV", "") == "1",
	}

	port, err := strconv.Atoi(env("AFTERFEED_PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("AFTERFEED_PORT must be a port number")
	}
	c.Port = port

	c.CacheDir = env("AFTERFEED_CACHE_DIR", "")
	if c.CacheDir == "" {
		// A separate /cache volume is optional. Use it when it is mounted,
		// otherwise keep the cache inside the config volume.
		if fi, err := os.Stat("/cache"); err == nil && fi.IsDir() {
			c.CacheDir = "/cache"
		} else {
			c.CacheDir = filepath.Join(c.ConfigDir, "cache")
		}
	}
	c.ImportDir = env("AFTERFEED_IMPORT_DIR", filepath.Join(c.DataDir, "import"))

	switch c.SecureCookies {
	case "auto", "true", "false":
	default:
		return nil, fmt.Errorf("AFTERFEED_SECURE_COOKIES must be auto, true or false")
	}
	return c, nil
}

// EnsureDirs creates the directories Afterfeed writes to.
func (c *Config) EnsureDirs() error {
	for _, dir := range []string{
		c.ConfigDir,
		c.CacheDir,
		c.DataDir,
		c.ImportDir,
		c.MemoriesDir(),
		c.MessagesDir(),
		c.StagingDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// DatabasePath is the SQLite database file.
func (c *Config) DatabasePath() string { return filepath.Join(c.ConfigDir, "afterfeed.db") }

// MemoriesDir holds one folder per memory with its media and a memory.json file.
func (c *Config) MemoriesDir() string { return filepath.Join(c.DataDir, "memories") }

// MessagesDir holds one folder per conversation.
func (c *Config) MessagesDir() string { return filepath.Join(c.DataDir, "messages") }

// StagingDir holds uploads in progress and archives being extracted. It lives
// on the data volume so finished files can be moved instead of copied.
func (c *Config) StagingDir() string { return filepath.Join(c.DataDir, ".afterfeed", "staging") }

// Addr is the listen address.
func (c *Config) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
