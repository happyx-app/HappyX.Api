package http_test

import (
	"strconv"
	"testing"
)

// TestTeamChannelAndNotes sets the team channel and the Monday summary, checks a deleted team
// channel goes back to the kudos channel, and goes through a note the bot publishes, edits and
// deletes.
func TestTeamChannelAndNotes(t *testing.T) {
	srv := newTestServer(t) // Thursday 22 October 2026
	const general, team = "500000000000000001", "500000000000000002"
	if code, out := do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+guildA+`","name":"A","admin_discord_user_id":"`+adminA+`"}`); code != 200 {
		t.Fatalf("register: %d %v", code, out)
	}
	if code, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); code != 200 || g["team_channel_id"] != "" || g["team_summary"] != true {
		t.Fatalf("defaults: %d %v", code, g)
	}
	do(t, srv, "PUT", "/v1/guilds/"+guildA+"/channels", key, `{"channels":[{"id":"`+general+`","name":"general","position":0,"can_post":true},{"id":"`+team+`","name":"equipo","position":1,"can_post":true}]}`)
	if code, _ := do(t, srv, "PATCH", "/v1/guilds/"+guildA, key, `{"team_channel_id":"500000000000000077"}`); code != 400 {
		t.Fatalf("unknown channel: %d, want 400", code)
	}
	if code, g := do(t, srv, "PATCH", "/v1/guilds/"+guildA, key, `{"team_channel_id":"`+team+`","team_summary":false}`); code != 200 || g["team_channel_id"] != team || g["team_summary"] != false {
		t.Fatalf("set: %d %v", code, g)
	}
	do(t, srv, "PUT", "/v1/guilds/"+guildA+"/channels", key, `{"channels":[{"id":"`+general+`","name":"general","position":0,"can_post":true}]}`)
	if _, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); g["team_channel_id"] != "" {
		t.Fatalf("deleted team channel kept: %v", g["team_channel_id"])
	}

	if code, _ := do(t, srv, "POST", "/v1/guilds/"+guildA+"/dispatches", key, `{"kind":"team_summary","period":"2026-10-19"}`); code != 200 && code != 201 {
		t.Fatalf("claim the summary: %d", code)
	}

	do(t, srv, "PUT", "/v1/guilds/"+guildA+"/members/"+adminA, key, `{"username":"alex"}`)
	body := `{"author_discord_user_id":"` + adminA + `","from":"2026-10-12","to":"2026-10-18","body":"Martes sin reuniones.","channel_id":"` + general + `","message_id":"600000000000000001"}`
	code, n := do(t, srv, "POST", "/v1/guilds/"+guildA+"/team-notes", key, body)
	if code != 201 || n["body"] != "Martes sin reuniones." || n["posted_on"] != "2026-10-22" || n["author_username"] != "alex" {
		t.Fatalf("publish: %d %v", code, n)
	}
	id := jsonID(n["id"])
	if code, _ := do(t, srv, "GET", "/v1/guilds/"+guildA+"/team-notes/current?day=2026-10-22", key, ""); code != 404 {
		t.Fatalf("current the same day: %d, want 404", code)
	}
	if code, c := do(t, srv, "GET", "/v1/guilds/"+guildA+"/team-notes/current?day=2026-10-23", key, ""); code != 200 || jsonID(c["id"]) != id {
		t.Fatalf("current the next day: %d %v", code, c)
	}
	if code, _ := do(t, srv, "POST", "/v1/guilds/"+guildA+"/team-notes", key, body); code != 409 {
		t.Fatalf("a second note for the same period: %d, want 409", code)
	}
	if code, c := do(t, srv, "GET", "/v1/guilds/"+guildA+"/team-notes?from=2026-10-12&to=2026-10-18", key, ""); code != 200 || jsonID(c["id"]) != id {
		t.Fatalf("note for the period: %d %v", code, c)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds/"+guildA+"/team-notes?from=2026-10-19&to=2026-10-25", key, ""); code != 404 {
		t.Fatalf("another period has no note: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds/"+guildA+"/team-notes?from=x&to=2026-10-25", key, ""); code != 400 {
		t.Fatalf("bad period: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds/"+guildA+"/team-notes/current?day=mañana", key, ""); code != 400 {
		t.Fatalf("bad day: %d", code)
	}
	if code, e := do(t, srv, "PATCH", "/v1/guilds/"+guildA+"/team-notes/"+id, key, `{"body":"Martes y jueves sin reuniones."}`); code != 200 || e["body"] != "Martes y jueves sin reuniones." {
		t.Fatalf("edit: %d %v", code, e)
	}
	if code, _ := do(t, srv, "DELETE", "/v1/guilds/"+guildB+"/team-notes/"+id, key, ""); code != 404 {
		t.Fatalf("delete from another guild: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", "/v1/guilds/"+guildA+"/team-notes/"+id, key, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds/"+guildA+"/team-notes/"+id, key, ""); code != 404 {
		t.Fatalf("deleted: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds/"+guildA+"/team-notes/abc", key, ""); code != 404 {
		t.Fatalf("bad id: %d", code)
	}
}

// jsonID turns a JSON number id into its path form.
func jsonID(v any) string {
	f, _ := v.(float64)
	return strconv.FormatInt(int64(f), 10)
}
