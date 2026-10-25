package http

import (
	"strings"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

func TestSession(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	v := domain.Viewer{DiscordID: "300000000000000001", Username: "Ana", ManagedGuilds: []string{"100000000000000001"}}
	tok := signSession(key, v, now.Add(time.Hour))

	got, err := verifySession(key, tok, now)
	if err != nil || got.DiscordID != v.DiscordID || got.Username != "Ana" || len(got.ManagedGuilds) != 1 {
		t.Fatalf("valid session: %+v, %v", got, err)
	}
	if _, err := verifySession(key, tok, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired session accepted")
	}
	if _, err := verifySession([]byte("another key, also 32 bytes long!"), tok, now); err == nil {
		t.Fatal("session signed with another key accepted")
	}
	forged := signSession(key, domain.Viewer{DiscordID: "300000000000000002"}, now.Add(time.Hour))
	body, _, _ := strings.Cut(forged, ".")
	_, sig, _ := strings.Cut(tok, ".")
	if _, err := verifySession(key, body+"."+sig, now); err == nil {
		t.Fatal("tampered session accepted")
	}
	for _, bad := range []string{"", "x", "x.y", "."} {
		if _, err := verifySession(key, bad, now); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
