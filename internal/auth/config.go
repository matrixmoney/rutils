package auth

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ErrConfigNotFound is returned by LoadConfig when no config.json could be
// located on any of the searched paths. Callers can use errors.Is(err,
// ErrConfigNotFound) to distinguish "file missing" from "file malformed".
var ErrConfigNotFound = errors.New("config.json not found on any search path")

// Config holds application configuration for auth and mail services
type Config struct {
	SMTPEmail          string `json:"smtp_email"`
	SMTPAppPassword    string `json:"smtp_app_password"`
	SMTPHost           string `json:"smtp_host"`
	SMTPPort           string `json:"smtp_port"`
	SMTPSenderName     string `json:"smtp_sender_name"`
	JWTSecret          string `json:"jwt_secret"`
	JWTExpirationHours int    `json:"jwt_expiration_hours"`
	DataDir            string `json:"data_dir"`
}

// DefaultConfig returns safe production-ready defaults
func DefaultConfig() *Config {
	return &Config{
		SMTPEmail:          "",
		SMTPAppPassword:    "",
		SMTPHost:           "smtp.gmail.com",
		SMTPPort:           "587",
		SMTPSenderName:     "Rutils Team",
		JWTSecret:          "rutils-utility-jwt-secret-change-in-production-2026",
		JWTExpirationHours: 24,
		DataDir:            "data",
	}
}

// IsSMTPConfigured returns true if SMTP credentials are fully provided
func (c *Config) IsSMTPConfigured() bool {
	return c != nil &&
		strings.TrimSpace(c.SMTPEmail) != "" &&
		strings.TrimSpace(c.SMTPAppPassword) != ""
}

// LoadConfig reads configuration from path or falls back to defaults.
//
// Behaviour:
//   - If a config file is found and parses correctly, LoadConfig returns
//     (cfg, nil) with the parsed values applied.
//   - If a config file is found but fails to parse, LoadConfig returns
//     (nil, parse-error). The caller must decide whether to abort.
//   - If NO config file is found on any search path, LoadConfig returns
//     (DefaultConfig(), ErrConfigNotFound). The caller can inspect this
//     with errors.Is() to log a loud warning — the returned *Config is
//     always non-nil so callers that ignore the error still get working
//     defaults.
//
// The function trims whitespace and stray quote characters from
// SMTPEmail and SMTPAppPassword. Gmail displays app passwords as
// "abcd efgh ijkl mnop" (with spaces); users paste them verbatim, and
// the SMTP AUTH command then fails with 535-5.7.8. Stripping spaces
// here makes copy-paste work correctly.
func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// ------------------------------------------------------------
	// Build the search path list, in priority order.
	//
	// $PWD/config.json is tried FIRST because the two documented ways
	// to run the server are:
	//   1. `./dev.sh`               (cwd = repo root)
	//   2. `go run ./cmd/server/main.go` (cwd = repo root)
	//
	// For deployments where the binary is executed from cmd/server/,
	// the ../config.json entries catch it. For deployments where the
	// binary is executed from an arbitrary directory (systemd, Docker),
	// the executable-relative entries catch it.
	// ------------------------------------------------------------
	var searchPaths []string
	if configPath != "" {
		searchPaths = append(searchPaths, configPath)
	}
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		searchPaths = append(searchPaths, envPath)
	}
	searchPaths = append(searchPaths,
		"config.json",                       // cwd (repo root)
		filepath.Join("cmd", "server", "config.json"), // new: run from repo root, config in cmd/server/
		filepath.Join("rutils", "config.json"),
		filepath.Join("..", "config.json"),        // run from cmd/server/ or a subdir
		filepath.Join("..", "..", "config.json"),  // run from cmd/server/somewhere
	)

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		searchPaths = append(searchPaths,
			filepath.Join(exeDir, "config.json"),
			filepath.Join(exeDir, "..", "config.json"),
			filepath.Join(exeDir, "..", "..", "config.json"),
			filepath.Join(exeDir, "..", "..", "..", "config.json"),
		)
	}

	var foundPath string
	var data []byte
	for _, p := range searchPaths {
		if content, err := os.ReadFile(p); err == nil {
			foundPath = p
			data = content
			break
		}
	}

	// ------------------------------------------------------------
	// No config file was found anywhere.
	// ------------------------------------------------------------
	if data == nil {
		log.Printf(
			"[AUTH-WARN] config.json not found. Searched: %v\n"+
				"            Falling back to DefaultConfig(). SMTP is DISABLED; "+
				"temporary passwords and task notifications will only be logged to stdout, not emailed.",
			searchPaths,
		)
		return cfg, ErrConfigNotFound
	}

	// ------------------------------------------------------------
	// Config file found — parse it.
	// ------------------------------------------------------------
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// ------------------------------------------------------------
	// Sanitise whitespace and stray quotes on the SMTP credentials.
	//
	// Google displays the app password as "abcd efgh ijkl mnop";
	// users copy-paste verbatim. SMTP AUTH rejects the whole string.
	// Strip spaces and surrounding quotes so the 16-char token is used.
	// ------------------------------------------------------------
	cfg.SMTPEmail = strings.TrimSpace(strings.Trim(cfg.SMTPEmail, `"'`))
	cfg.SMTPAppPassword = strings.ReplaceAll(
		strings.TrimSpace(strings.Trim(cfg.SMTPAppPassword, `"'`)),
		" ", "",
	)

	// ------------------------------------------------------------
	// Apply sensible fallbacks for any missing non-secret fields.
	// ------------------------------------------------------------
	if cfg.SMTPHost == "" {
		cfg.SMTPHost = "smtp.gmail.com"
	}
	if cfg.SMTPPort == "" {
		cfg.SMTPPort = "587"
	}
	if cfg.SMTPSenderName == "" {
		cfg.SMTPSenderName = "Rutils Team"
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "rutils-utility-jwt-secret-change-in-production-2026"
	}
	if cfg.JWTExpirationHours <= 0 {
		cfg.JWTExpirationHours = 24
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}

	// ------------------------------------------------------------
	// Log a single, explicit line at INFO so the operator can see
	// which file was used and whether SMTP is live.
	// ------------------------------------------------------------
	if cfg.IsSMTPConfigured() {
		log.Printf(
			"[AUTH] config.json resolved to %q — SMTP ACTIVE for sender <%s> via %s:%s",
			foundPath, cfg.SMTPEmail, cfg.SMTPHost, cfg.SMTPPort,
		)
	} else {
		log.Printf(
			"[AUTH-WARN] config.json resolved to %q — SMTP NOT configured "+
				"(smtp_email or smtp_app_password is empty). "+
				"Emails will be logged to stdout instead of dispatched.",
			foundPath,
		)
	}

	return cfg, nil
}

// GetDataDir returns resolved data directory path
func (c *Config) GetDataDir() string {
	dir := c.DataDir
	if dir == "" {
		dir = "data"
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		alt := filepath.Join("rutils", dir)
		if fi, err := os.Stat(alt); err == nil && fi.IsDir() {
			return alt
		}
	}
	return dir
}

// GetUsersFilePath returns the resolved path to data/users.json
func (c *Config) GetUsersFilePath() string {
	return filepath.Join(c.GetDataDir(), "users.json")
}