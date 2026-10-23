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
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
