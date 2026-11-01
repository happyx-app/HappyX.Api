package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

// The survey links the privacy policy until a member answers on the policy's date or later, in
// that guild: members who answered only before it see the link too.
func TestMembersPolicyPending(t *testing.T) {
	policy := usecase.PrivacyPolicyDate
	svc := testService(t, policy.AddDate(0, 0, 3).Add(12*time.Hour))
	ctx := context.Background()
	mustRegister(t, svc, guildA)
	mustRegister(t, svc, guildB)
	const (
		before = "200000000000000001" // answered only before the policy
		after  = "200000000000000002" // answered on the policy's day
		never  = "200000000000000003"
		other  = "200000000000000004" // answered after it, but in the other guild
	)
	for _, u := range []string{before, after, never, other} {
		mustJoin(t, svc, guildA, u)
	}
	mustJoin(t, svc, guildB, other)
	answer := func(guild, user string, day time.Time) {
		if _, err := svc.RecordMood(ctx, usecase.RecordMoodInput{GuildID: guild, UserID: user, Mood: domain.MoodGood, Day: day}); err != nil {
			t.Fatal(err)
		}
	}
	answer(guildA, before, policy.AddDate(0, 0, -1))
	answer(guildA, after, policy)
	answer(guildB, other, policy.AddDate(0, 0, 1))

	ms, err := svc.ListMembers(ctx, guildA, usecase.MembersSurvey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.MembersPolicyPending(ctx, guildA, ms)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{before: true, never: true, other: true}
	if len(got) != len(want) {
		t.Fatalf("pending = %v, want %v", got, want)
	}
	for u := range want {
		if !got[u] {
			t.Fatalf("pending = %v, want %v", got, want)
		}
	}
}
