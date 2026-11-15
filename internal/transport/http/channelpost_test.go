package http_test

import "testing"

// TestChannelPosts stores where the bot keeps the survey for whoever doesn't get its DMs, moves
// it, and forgets it.
func TestChannelPosts(t *testing.T) {
	srv := newTestServer(t)
	do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+guildA+`","name":"A","admin_discord_user_id":"`+adminA+`"}`)
	path := "/v1/guilds/" + guildA + "/channel-posts/survey"
	if code, _ := do(t, srv, "GET", path, key, ""); code != 404 {
		t.Fatalf("none yet: %d", code)
	}
	if code, _ := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/channel-posts/banner", key, `{"channel_id":"500000000000000001","message_id":"600000000000000001"}`); code != 400 {
		t.Fatalf("unknown kind: %d, want 400", code)
	}
	if code, _ := do(t, srv, "PUT", path, key, `{"channel_id":"general","message_id":"600000000000000001"}`); code != 400 {
		t.Fatalf("bad channel: %d, want 400", code)
	}
	if code, _ := do(t, srv, "PUT", "/v1/guilds/"+guildB+"/channel-posts/survey", key, `{"channel_id":"500000000000000001","message_id":"600000000000000001"}`); code != 404 {
		t.Fatalf("unknown guild: %d, want 404", code)
	}
	if code, p := do(t, srv, "PUT", path, key, `{"channel_id":"500000000000000001","message_id":"600000000000000001"}`); code != 200 || p["message_id"] != "600000000000000001" {
		t.Fatalf("put: %d %v", code, p)
	}
	if code, p := do(t, srv, "PUT", path, key, `{"channel_id":"500000000000000002","message_id":"600000000000000002"}`); code != 200 || p["channel_id"] != "500000000000000002" {
		t.Fatalf("move: %d %v", code, p)
	}
	if code, p := do(t, srv, "GET", path, key, ""); code != 200 || p["kind"] != "survey" || p["channel_id"] != "500000000000000002" || p["message_id"] != "600000000000000002" {
		t.Fatalf("get: %d %v", code, p)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds/"+guildB+"/channel-posts/survey", key, ""); code != 404 {
		t.Fatalf("another guild's: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", path, key, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", path, key, ""); code != 404 {
		t.Fatalf("delete again: %d", code)
	}
}

// TestMemberMoodsAndGuilds reads a member's own week in one server, and the servers a user is in.
func TestMemberMoodsAndGuilds(t *testing.T) {
	srv := newTestServer(t) // Thursday 22 October 2026
	for _, g := range []string{guildA, guildB} {
		do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+g+`","name":"`+g[len(g)-1:]+`","admin_discord_user_id":"`+adminA+`"}`)
		do(t, srv, "PUT", "/v1/guilds/"+g+"/members/"+memberID, key, `{"username":"ana"}`)
	}
	do(t, srv, "PUT", "/v1/guilds/"+guildA+"/members/"+memberID+"/mood", key, `{"mood":4,"day":"2026-10-21","message":"Bien"}`)
	do(t, srv, "PUT", "/v1/guilds/"+guildA+"/members/"+memberID+"/mood", key, `{"mood":2}`)
	do(t, srv, "PUT", "/v1/guilds/"+guildB+"/members/"+memberID+"/mood", key, `{"mood":5}`)

	moods := "/v1/guilds/" + guildA + "/members/" + memberID + "/moods"
	code, ms := doList(t, srv, "GET", moods+"?from=2026-10-19&to=2026-10-25", "")
	if code != 200 || len(ms) != 2 || ms[0]["day"] != "2026-10-21" || ms[0]["message"] != "Bien" || ms[1]["mood"] != float64(2) {
		t.Fatalf("week: %d %v", code, ms)
	}
	if code, ms := doList(t, srv, "GET", moods+"?from=2026-10-22&to=2026-10-22", ""); code != 200 || len(ms) != 1 {
		t.Fatalf("one day: %d %v", code, ms)
	}
	if code, _ := do(t, srv, "GET", moods+"?from=2026-10-01&to=2026-11-01", key, ""); code != 400 {
		t.Fatalf("more than 31 days: %d", code)
	}
	if code, _ := do(t, srv, "GET", moods+"?from=2026-10-25&to=2026-10-19", key, ""); code != 400 {
		t.Fatalf("backwards: %d", code)
	}
	if code, _ := do(t, srv, "GET", moods+"?from=lunes&to=2026-10-19", key, ""); code != 400 {
		t.Fatalf("bad date: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds/"+guildA+"/members/200000000000000099/moods?from=2026-10-19&to=2026-10-25", key, ""); code != 404 {
		t.Fatalf("not a member: %d", code)
	}

	if code, gs := doList(t, srv, "GET", "/v1/users/"+memberID+"/guilds", ""); code != 200 || len(gs) != 2 || gs[0]["discord_guild_id"] != guildA || gs[0]["paused"] != false {
		t.Fatalf("user guilds: %d %v", code, gs)
	}
	do(t, srv, "DELETE", "/v1/guilds/"+guildB+"/members/"+memberID, key, "") // paused: still in the server
	if code, gs := doList(t, srv, "GET", "/v1/users/"+memberID+"/guilds", ""); code != 200 || len(gs) != 2 || gs[0]["paused"] != false || gs[1]["paused"] != true {
		t.Fatalf("after pausing one: %d %v", code, gs)
	}
	do(t, srv, "DELETE", "/v1/guilds/"+guildB+"/members/"+memberID+"?left=true", key, "")
	if code, gs := doList(t, srv, "GET", "/v1/users/"+memberID+"/guilds", ""); code != 200 || len(gs) != 1 {
		t.Fatalf("after leaving one: %d %v", code, gs)
	}
	if code, _ := do(t, srv, "GET", "/v1/users/ana/guilds", key, ""); code != 400 {
		t.Fatalf("bad user: %d", code)
	}
}
