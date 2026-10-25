package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// A session is the logged-in viewer, signed with HMAC-SHA256 and kept in an HttpOnly cookie,
// so the API stores nothing per session. Discord permissions are a snapshot from login; the
// session's lifetime bounds how stale they can get.

type sessionClaims struct {
	Sub     string   `json:"sub"`
	Name    string   `json:"name"`
	Avatar  string   `json:"avatar,omitempty"`
	Guilds  []string `json:"guilds,omitempty"`
	Expires int64    `json:"exp"`
}

var errBadSession = errors.New("invalid session")

func signSession(key []byte, v domain.Viewer, expires time.Time) string {
	payload, _ := json.Marshal(sessionClaims{
		Sub: v.DiscordID, Name: v.Username, Avatar: v.Avatar, Guilds: v.ManagedGuilds, Expires: expires.Unix(),
	})
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + base64.RawURLEncoding.EncodeToString(mac(key, body))
}

func verifySession(key []byte, token string, now time.Time) (domain.Viewer, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return domain.Viewer{}, errBadSession
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, mac(key, body)) {
		return domain.Viewer{}, errBadSession
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return domain.Viewer{}, errBadSession
	}
	var c sessionClaims
	if err := json.Unmarshal(payload, &c); err != nil || c.Sub == "" || now.Unix() >= c.Expires {
		return domain.Viewer{}, errBadSession
	}
	return domain.Viewer{DiscordID: c.Sub, Username: c.Name, Avatar: c.Avatar, ManagedGuilds: c.Guilds}, nil
}

func mac(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(msg))
	return h.Sum(nil)
}
