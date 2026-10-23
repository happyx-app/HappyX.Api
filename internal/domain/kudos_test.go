package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestKudosValidate(t *testing.T) {
	ok := Kudos{GuildDiscordID: guildA, GiverDiscordID: userA, ReceiverDiscordID: userB, Day: day("2026-10-20"), Message: "gracias"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid kudos rejected: %v", err)
	}
	cases := map[string]func(*Kudos){
		"to yourself":      func(k *Kudos) { k.ReceiverDiscordID = userA },
		"empty message":    func(k *Kudos) { k.Message = "  " },
		"message too long": func(k *Kudos) { k.Message = strings.Repeat("a", MaxKudosLength+1) },
		"bad receiver":     func(k *Kudos) { k.ReceiverDiscordID = "abc" },
		"missing day":      func(k *Kudos) { k.Day = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			k := ok
			mutate(&k)
			if err := k.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestSummarizeKudos(t *testing.T) {
	var ks []Kudos
	for i := 0; i < 7; i++ {
		ks = append(ks, Kudos{GiverDiscordID: userA, ReceiverDiscordID: userB, Message: string(rune('a' + i))})
	}
	ks = append(ks, Kudos{GiverDiscordID: userB, ReceiverDiscordID: userA, Message: "last"})
	s := SummarizeKudos(ks)
	if s.Count != 8 || s.Givers != 2 || s.Receivers != 2 {
		t.Fatalf("counts: %+v", s)
	}
	if len(s.Highlights) != MaxKudosHighlights || s.Highlights[0].Message != "last" || s.Highlights[1].Message != "g" {
		t.Fatalf("highlights should be the latest, newest first: %+v", s.Highlights)
	}
	if empty := SummarizeKudos(nil); empty.Count != 0 || empty.Highlights == nil {
		t.Fatalf("empty summary: %+v", empty)
	}
}

func TestKudosChannelSetting(t *testing.T) {
	s := DefaultGuildSettings()
	s.KudosChannelID = "400000000000000001"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.KudosChannelID = "general"
	if err := s.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}
