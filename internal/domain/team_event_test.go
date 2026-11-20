package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTeamEventValidate(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
	today := day("2026-11-20")
	ok := TeamEvent{Title: "Hack", Color: "red", From: day("2026-11-03"), To: day("2026-11-05")}
	if err := ok.Validate(today); err != nil {
		t.Fatalf("valid event: %v", err)
	}
	if err := (TeamEvent{Title: "Dinner", Color: "yellow", From: today, To: today}).Validate(today); err != nil {
		t.Fatalf("an event today is fine: %v", err)
	}
	for name, e := range map[string]TeamEvent{
		"no title":      {Color: "red", From: ok.From, To: ok.To},
		"bad color":     {Title: "x", Color: "violet", From: ok.From, To: ok.To},
		"long title":    {Color: "red", Title: strings.Repeat("a", MaxTeamEventTitle+1), From: ok.From, To: ok.To},
		"long text":     {Color: "red", Title: "x", Description: strings.Repeat("é", MaxTeamEventDescription+1), From: ok.From, To: ok.To},
		"no dates":      {Color: "red", Title: "x"},
		"reversed":      {Color: "red", Title: "x", From: ok.To, To: ok.From},
		"ends tomorrow": {Color: "red", Title: "x", From: today, To: today.AddDate(0, 0, 1)},
	} {
		if err := e.Validate(today); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
	if n := (TeamEvent{Title: "  Hack  ", Description: " d ", From: ok.From.Add(5 * time.Hour)}).Normalize(); n.Title != "Hack" || n.Description != "d" || !n.From.Equal(ok.From) || n.Color != "yellow" {
		t.Errorf("normalize: %+v", n)
	}
}
