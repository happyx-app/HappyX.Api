package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestViewer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if id, secret, _ := r.BasicAuth(); id != "cid" || secret != "secret" || r.FormValue("code") != "abc" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /users/@me", auth(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"id":"300000000000000001","username":"ana","global_name":"Ana","avatar":"a1"}`))
	}))
	mux.HandleFunc("GET /users/@me/guilds", auth(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[
			{"id":"100000000000000001","owner":true,"permissions":"0"},
			{"id":"100000000000000002","owner":false,"permissions":"8"},
			{"id":"100000000000000003","owner":false,"permissions":"2251799813685247"},
			{"id":"100000000000000004","owner":false,"permissions":"1024"}
		]`))
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	o := NewOAuth("cid", "secret", "https://example.com/cb")
	o.BaseURL = srv.URL
	v, err := o.Viewer(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if v.DiscordID != "300000000000000001" || v.Username != "Ana" || v.Avatar != "a1" {
		t.Fatalf("viewer: %+v", v)
	}
	want := []string{"100000000000000001", "100000000000000002", "100000000000000003"}
	if !slices.Equal(v.ManagedGuilds, want) {
		t.Fatalf("managed guilds: got %v, want %v", v.ManagedGuilds, want)
	}
	if _, err := o.Viewer(context.Background(), "wrong"); err == nil {
		t.Fatal("a bad code should fail")
	}
}
