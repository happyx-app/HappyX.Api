package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

func TestGameRound(t *testing.T) {
	ctx := context.Background()
	// Monday 2026-10-19 in Madrid.
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	svc := testServiceClock(t, func() time.Time { return now })
	const user3, user4 = "200000000000000003", "200000000000000004"
	mustRegister(t, svc, guildA)
	mustRegister(t, svc, guildB)
	for _, u := range []string{user1, user2, user3, user4} {
		mustJoin(t, svc, guildA, u)
	}
	mustJoin(t, svc, guildB, user1)
	week := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)

	if _, err := svc.StartGameRound(ctx, guildA, week, "¿Qué querías ser de pequeño?"); !errors.Is(err, domain.ErrGamesOff) {
		t.Fatalf("games off: want ErrGamesOff, got %v", err)
	}
	on := true
	if _, err := svc.UpdateGuildSettings(ctx, guildA, usecase.SettingsPatch{GamesEnabled: &on}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartGameRound(ctx, guildA, week, "¿Qué querías ser de pequeño?"); err != nil {
		t.Fatal(err)
	}
	// Retrying keeps the first question; other weeks are refused.
	if r, err := svc.StartGameRound(ctx, guildA, week, "Otra"); err != nil || r.Question != "¿Qué querías ser de pequeño?" {
		t.Fatalf("restart: %+v %v", r, err)
	}
	if _, err := svc.StartGameRound(ctx, guildA, week.AddDate(0, 0, 1), "x"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("not a Monday: %v", err)
	}
	for _, w := range []time.Time{week.AddDate(0, 0, -7), week.AddDate(0, 0, 7)} {
		if _, err := svc.StartGameRound(ctx, guildA, w, "x"); !errors.Is(err, domain.ErrClosed) {
			t.Fatalf("week %s: want ErrClosed, got %v", w, err)
		}
	}

	answer := func(guild, user, text string) error {
		_, err := svc.AnswerGame(ctx, usecase.AnswerGameInput{GuildID: guild, UserID: user, Text: text})
		return err
	}
	for u, text := range map[string]string{user1: "Bombero", user2: "Astronauta", user3: "Pastelera", user4: "Piloto"} {
		if err := answer(guildA, u, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := answer(guildA, user1, "Bombera"); err != nil { // changing it is fine
		t.Fatal(err)
	}
	if err := answer(guildB, user1, "x"); !errors.Is(err, domain.ErrGamesOff) {
		t.Fatalf("guild B has games off: %v", err)
	}
	if err := answer(guildA, admin, "x"); !errors.Is(err, domain.ErrNotAMember) {
		t.Fatalf("not a member: %v", err)
	}

	v1 := domain.Viewer{DiscordID: user1}
	b, err := svc.GameBoard(ctx, v1, guildA)
	if err != nil {
		t.Fatal(err)
	}
	if c := b.Current; c == nil || c.Phase != domain.GameAnswering || c.Count != 4 || len(c.Answers) != 0 || c.MyAnswer.Text != "Bombera" || b.Previous != nil {
		t.Fatalf("answering board: %+v", b)
	}
	if err := svc.GuessGame(ctx, v1, guildA, map[string]string{}); !errors.Is(err, domain.ErrClosed) {
		t.Fatalf("guessing before Thursday: %v", err)
	}

	// Thursday: answers close, guessing opens. user4 paused, so their answer leaves the game.
	now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	if err := answer(guildA, user2, "x"); !errors.Is(err, domain.ErrClosed) {
		t.Fatalf("answering on Thursday: %v", err)
	}
	if err := svc.LeaveMember(ctx, guildA, user4, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GameBoard(ctx, domain.Viewer{DiscordID: user4}, guildA); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("paused member sees the game: %v", err)
	}
	b, err = svc.GameBoard(ctx, v1, guildA)
	if err != nil {
		t.Fatal(err)
	}
	c := b.Current
	if c.Phase != domain.GameGuessing || c.Count != 3 || len(c.Answers) != 3 || len(c.Players) != 3 {
		t.Fatalf("guessing board: %+v", c)
	}
	ids := map[string]string{} // answer text -> ID
	for _, a := range c.Answers {
		ids[a.Text] = a.ID
		if mine := a.Text == "Bombera"; (a.MemberDiscordID != "") != mine {
			t.Fatalf("author shown while guessing: %+v", a)
		}
	}
	for name, guesses := range map[string]map[string]string{
		"own answer":     {ids["Bombera"]: user2},
		"naming oneself": {ids["Astronauta"]: user1},
		"unknown answer": {"00000000-0000-0000-0000-000000000000": user2},
		"not a player":   {ids["Astronauta"]: user4},
	} {
		if err := svc.GuessGame(ctx, v1, guildA, guesses); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	if err := svc.GuessGame(ctx, v1, guildA, map[string]string{ids["Astronauta"]: user3, ids["Pastelera"]: user3}); err != nil {
		t.Fatal(err)
	}
	// Guessing again replaces the earlier guesses.
	if err := svc.GuessGame(ctx, v1, guildA, map[string]string{ids["Astronauta"]: user2, ids["Pastelera"]: user2}); err != nil {
		t.Fatal(err)
	}
	if err := svc.GuessGame(ctx, domain.Viewer{DiscordID: user2}, guildA, map[string]string{ids["Bombera"]: user1, ids["Pastelera"]: user3}); err != nil {
		t.Fatal(err)
	}

	// Next Monday the round is over: authors and ranking come out.
	now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	b, err = svc.GameBoard(ctx, v1, guildA)
	if err != nil {
		t.Fatal(err)
	}
	p := b.Previous
	if b.Current != nil || p == nil || p.Phase != domain.GameFinished || len(p.Answers) != 3 {
		t.Fatalf("finished board: %+v", b)
	}
	for _, a := range p.Answers {
		if a.MemberDiscordID == "" || a.Username == "" {
			t.Fatalf("author hidden after the round: %+v", a)
		}
	}
	if p.Right[ids["Pastelera"]] != 1 || p.Right[ids["Astronauta"]] != 1 || p.Right[ids["Bombera"]] != 1 {
		t.Fatalf("right guesses: %v", p.Right)
	}
	if len(p.Ranking) != 2 || p.Ranking[0].DiscordID != user2 || p.Ranking[0].Correct != 2 || p.Ranking[1].Correct != 1 {
		t.Fatalf("ranking: %+v", p.Ranking)
	}

	// Guild B can't see guild A's game, and only its members can play there.
	if _, err := svc.UpdateGuildSettings(ctx, guildB, usecase.SettingsPatch{GamesEnabled: &on}); err != nil {
		t.Fatal(err)
	}
	if b, err := svc.GameBoard(ctx, v1, guildB); err != nil || b.Current != nil || b.Previous != nil {
		t.Fatalf("guild B board: %+v %v", b, err)
	}
	if _, err := svc.GameBoard(ctx, domain.Viewer{DiscordID: user2}, guildB); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member of B: %v", err)
	}
	gs, err := svc.GameGuilds(ctx, v1)
	if err != nil || len(gs) != 2 {
		t.Fatalf("game guilds for user1: %v %v", gs, err)
	}
	if gs, _ := svc.GameGuilds(ctx, domain.Viewer{DiscordID: user4}); len(gs) != 0 {
		t.Fatalf("paused member gets games: %v", gs)
	}

	// Last week's round is in Previous, not in the history yet; nor in «My evolution».
	if len(b.History) != 0 {
		t.Fatalf("history with only last week's round: %+v", b.History)
	}
	// A round that only one person answered.
	if _, err := svc.StartGameRound(ctx, guildA, time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), "¿Playa o montaña?"); err != nil {
		t.Fatal(err)
	}
	if err := answer(guildA, user1, "Montaña"); err != nil {
		t.Fatal(err)
	}

	// A week on, the first round moves to the history.
	now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	b, err = svc.GameBoard(ctx, v1, guildA)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.History) != 1 || !b.History[0].Round.Week.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("history on 2 Nov: %+v", b.History)
	}
	if b.Previous == nil || b.Previous.Enough {
		t.Fatalf("last week's round with one answer: %+v", b.Previous)
	}
	// Another week on, both rounds are in the history, newest first.
	now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	b, err = svc.GameBoard(ctx, v1, guildA)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.History) != 2 {
		t.Fatalf("history on 9 Nov: %+v", b.History)
	}
	notPlayed, played := b.History[0], b.History[1]
	if notPlayed.Enough || notPlayed.Count != 1 || len(notPlayed.Answers) != 0 || notPlayed.Guesses != 0 {
		t.Fatalf("round with one answer: %+v", notPlayed)
	}
	// user1 got one of two right; with user2's two right ones, the team got three of four.
	if !played.Enough || played.Count != 3 || len(played.Answers) != 3 || played.Guesses != 4 || played.Correct != 3 ||
		played.MyGuesses != 2 || played.MyCorrect != 1 || played.MyGuessed[ids["Pastelera"]] != user2 || played.MyAnswerID != ids["Bombera"] {
		t.Fatalf("played round: %+v", played)
	}
	if played.Right[ids["Bombera"]] != 1 || played.Right[ids["Pastelera"]] != 1 {
		t.Fatalf("history right guesses: %v", played.Right)
	}
	// user3 didn't guess: no score of their own, the same team total.
	b3, err := svc.GameBoard(ctx, domain.Viewer{DiscordID: user3}, guildA)
	if err != nil || len(b3.History) != 2 || b3.History[1].MyGuesses != 0 || len(b3.History[1].MyGuessed) != 0 || b3.History[1].Correct != 3 {
		t.Fatalf("user3's history: %+v %v", b3.History, err)
	}

	// «My evolution»: only finished rounds the viewer guessed in, per guild, and no other guild's.
	mine, err := svc.MyGames(ctx, v1, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC))
	if err != nil || len(mine) != 2 {
		t.Fatalf("my games: %+v %v", mine, err)
	}
	for _, m := range mine {
		switch m.Guild.DiscordID {
		case guildA:
			if len(m.Weeks) != 1 || m.Weeks[0].Correct != 1 || m.Weeks[0].Guesses != 2 {
				t.Fatalf("my games in A: %+v", m.Weeks)
			}
		case guildB:
			if len(m.Weeks) != 0 {
				t.Fatalf("my games in B: %+v", m.Weeks)
			}
		}
	}
	if mine, _ := svc.MyGames(ctx, v1, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)); len(mine[0].Weeks)+len(mine[1].Weeks) != 0 {
		t.Fatalf("a range that starts after the round: %+v", mine)
	}
	if _, err := svc.MyGames(ctx, v1, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("backwards range: %v", err)
	}
}
