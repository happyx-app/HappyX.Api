// Package discord logs dashboard users in with Discord OAuth2 (scopes identify and guilds).
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

const permAdministrator = 1 << 3

type OAuth struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// BaseURL is Discord's API root; tests point it at a fake server.
	BaseURL string
	HTTP    *http.Client
}

func NewOAuth(clientID, clientSecret, redirectURL string) *OAuth {
	return &OAuth{
		ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
		BaseURL: "https://discord.com/api/v10",
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// AuthURL is where the browser is sent to approve the login.
func (o *OAuth) AuthURL(state string) string {
	q := url.Values{
		"client_id":     {o.ClientID},
		"redirect_uri":  {o.RedirectURL},
		"response_type": {"code"},
		"scope":         {"identify guilds"},
		"state":         {state},
		"prompt":        {"none"},
	}
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}

// botPermissions are what the bot asks for when it's added: View Channel, Send Messages and
// Embed Links, so kudos can be posted in a kudos channel. Surveys and reports go by DM.
const botPermissions = 1<<10 | 1<<11 | 1<<14

// InviteURL adds the bot to a server. The dashboard and the bot share one Discord application.
func (o *OAuth) InviteURL() string {
	q := url.Values{
		"client_id":   {o.ClientID},
		"scope":       {"bot applications.commands"},
		"permissions": {strconv.Itoa(botPermissions)},
	}
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}

// Viewer exchanges the authorization code and reads who the user is and which servers they
// manage. The Discord token is used only here and never stored.
func (o *OAuth) Viewer(ctx context.Context, code string) (domain.Viewer, error) {
	token, err := o.exchange(ctx, code)
	if err != nil {
		return domain.Viewer{}, err
	}
	var user struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
		Avatar     string `json:"avatar"`
	}
	if err := o.get(ctx, token, "/users/@me", &user); err != nil {
		return domain.Viewer{}, err
	}
	var guilds []struct {
		ID          string `json:"id"`
		Owner       bool   `json:"owner"`
		Permissions string `json:"permissions"`
	}
	if err := o.get(ctx, token, "/users/@me/guilds", &guilds); err != nil {
		return domain.Viewer{}, err
	}
	v := domain.Viewer{DiscordID: user.ID, Username: user.GlobalName, Avatar: user.Avatar}
	if v.Username == "" {
		v.Username = user.Username
	}
	for _, g := range guilds {
		perms, _ := strconv.ParseUint(g.Permissions, 10, 64)
		if g.Owner || perms&permAdministrator != 0 {
			v.ManagedGuilds = append(v.ManagedGuilds, g.ID)
		}
	}
	return v, nil
}

func (o *OAuth) exchange(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {o.RedirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(o.ClientID, o.ClientSecret)
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := o.do(req, &out); err != nil {
		return "", fmt.Errorf("discord token exchange: %w", err)
	}
	return out.AccessToken, nil
}

func (o *OAuth) get(ctx context.Context, token, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if err := o.do(req, v); err != nil {
		return fmt.Errorf("discord %s: %w", path, err)
	}
	return nil
}

func (o *OAuth) do(req *http.Request, v any) error {
	res, err := o.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}
