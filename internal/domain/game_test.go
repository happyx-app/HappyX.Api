package domain

import (
	"testing"
	"time"
)

func TestGamePhases(t *testing.T) {
	week := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	r := GameRound{Week: week}
	for day, want := range map[int]GamePhase{
		19: GameAnswering, 21: GameAnswering, 22: GameGuessing, 25: GameGuessing, 26: GameFinished,
	} {
		d := time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC)
		if got := r.Phase(d); got != want {
			t.Errorf("Oct %d: got %s, want %s", day, got, want)
		}
		if day < 26 && !WeekStart(d).Equal(week) {
			t.Errorf("WeekStart(Oct %d) = %s", day, WeekStart(d))
		}
	}
}

func TestRanking(t *testing.T) {
	answers := []GameAnswer{{ID: "a", MemberDiscordID: "1"}, {ID: "b", MemberDiscordID: "2"}, {ID: "c", MemberDiscordID: "3"}}
	guesses := []GameGuess{
		{GuesserDiscordID: "1", AnswerID: "b", GuessedDiscordID: "2"},
		{GuesserDiscordID: "1", AnswerID: "c", GuessedDiscordID: "2"},
		{GuesserDiscordID: "2", AnswerID: "a", GuessedDiscordID: "1"},
		{GuesserDiscordID: "3", AnswerID: "a", GuessedDiscordID: "1"},
		{GuesserDiscordID: "3", AnswerID: "gone", GuessedDiscordID: "4"}, // answer no longer in the round
	}
	got := Ranking(answers, guesses, map[string]string{"1": "ana", "2": "bea", "3": "carla"})
	want := []GameScore{{"2", "bea", 1, 1}, {"3", "carla", 1, 1}, {"1", "ana", 1, 2}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	}
}
