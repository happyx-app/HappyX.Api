// Package config loads settings from environment variables. Secrets are never read from files
// in the repo; in development use a git-ignored .env loaded by docker compose.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Addr        string
	DatabaseURL string
	// ServiceKeyHashes are SHA-256 hex digests of the API keys allowed to call the API
	// (one per client, e.g. the bot). The plaintext keys are never stored by the API.
	ServiceKeyHashes [][]byte
	// Web is set when the dashboard login is configured (DISCORD_CLIENT_ID is not empty).
	Web *Web
	// Admin is set when the owner's admin dashboard API is on (ADMIN_KEY_HASH is not empty).
	Admin *Admin
}

// Admin is the owner's admin dashboard API, on a port of its own that is never published.
type Admin struct {
	Addr      string
	KeyHash   []byte // SHA-256 of the key HappyX.Admin sends
	BackupDir string // where the database backups are, read-only; empty hides them
}

type Web struct {
	PublicURL           string
	DiscordClientID     string
	DiscordClientSecret string
	SessionSecret       []byte
	// Demo serves the public demo server at /web/demo. On unless DEMO_ENABLED is "false".
	Demo bool
}

func Load() (Config, error) {
	c := Config{
		Addr:        getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	raw := os.Getenv("API_KEY_HASHES")
	for _, h := range strings.Split(raw, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		b, err := hex.DecodeString(h)
		if err != nil || len(b) != 32 {
			return c, fmt.Errorf("API_KEY_HASHES: %q is not a SHA-256 hex digest", h)
		}
		c.ServiceKeyHashes = append(c.ServiceKeyHashes, b)
	}
	if len(c.ServiceKeyHashes) == 0 {
		return c, errors.New("API_KEY_HASHES is required (comma separated SHA-256 hex digests)")
	}
	if raw := strings.TrimSpace(os.Getenv("ADMIN_KEY_HASH")); raw != "" {
		b, err := hex.DecodeString(raw)
		if err != nil || len(b) != 32 {
			return c, errors.New("ADMIN_KEY_HASH must be a SHA-256 hex digest")
		}
		for _, h := range c.ServiceKeyHashes {
			if string(h) == string(b) {
				return c, errors.New("ADMIN_KEY_HASH must not be one of API_KEY_HASHES: give the admin dashboard its own key")
			}
		}
		c.Admin = &Admin{Addr: getenv("ADMIN_ADDR", ":8081"), KeyHash: b, BackupDir: os.Getenv("BACKUP_DIR")}
		if c.Admin.Addr == c.Addr {
			return c, errors.New("ADMIN_ADDR must differ from HTTP_ADDR")
		}
	}
	if os.Getenv("DISCORD_CLIENT_ID") != "" {
		w := &Web{
			PublicURL:           strings.TrimRight(os.Getenv("WEB_URL"), "/"),
			DiscordClientID:     os.Getenv("DISCORD_CLIENT_ID"),
			DiscordClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
			SessionSecret:       []byte(os.Getenv("SESSION_SECRET")),
			Demo:                !strings.EqualFold(strings.TrimSpace(os.Getenv("DEMO_ENABLED")), "false"),
		}
		if !strings.HasPrefix(w.PublicURL, "http://") && !strings.HasPrefix(w.PublicURL, "https://") {
			return c, errors.New("WEB_URL is required with DISCORD_CLIENT_ID (e.g. https://happyx.example.com)")
		}
		if w.DiscordClientSecret == "" {
			return c, errors.New("DISCORD_CLIENT_SECRET is required with DISCORD_CLIENT_ID")
		}
		if len(w.SessionSecret) < 32 {
			return c, errors.New("SESSION_SECRET must be at least 32 characters (e.g. openssl rand -hex 32)")
		}
		c.Web = w
	}
	return c, nil
}

// RedirectURL is the OAuth2 redirect to register in the Discord application.
func (w Web) RedirectURL() string { return w.PublicURL + "/api/web/auth/callback" }

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
