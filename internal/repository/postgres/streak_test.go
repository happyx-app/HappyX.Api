package postgres

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

// The database's streak count must match domain.Streak, which the web's "my evolution" uses.
func TestMemberStreaksMatchDomain(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) // a Friday
	svc := testService(t, now)
	ctx := context.Background()
	mustRegister(t, svc, guildA)
	sunToThu := domain.Weekdays(1<<time.Sunday | 1<<time.Monday | 1<<time.Tuesday | 1<<time.Wednesday | 1<<time.Thursday)
	g, err := svc.UpdateGuildSettings(ctx, guildA, usecase.SettingsPatch{WorkingDays: &sunToThu})
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation(g.Settings.Timezone)
	today := domain.TruncateDay(now.In(loc))

	rng := rand.New(rand.NewSource(1))
	want := map[string]int{}
	for i := 0; i < 40; i++ {
		user := fmt.Sprintf("2000000000000%05d", i)
		mustJoin(t, svc, guildA, user)
		// Answer most days for a while back, with a gap somewhere, sometimes today, sometimes
		// on a weekend.
		answered := map[time.Time]bool{}
		chance := []float64{0, 0.5, 0.9, 1}[i%4]
		for d := 0; d < 30; d++ {
			day := today.AddDate(0, 0, -d)
			if rng.Float64() >= chance {
				continue
			}
			answered[day] = true
			if _, err := svc.RecordMood(ctx, usecase.RecordMoodInput{GuildID: guildA, UserID: user, Mood: domain.MoodGood, Day: day}); err != nil {
				t.Fatal(err)
			}
		}
		n := domain.Streak(g, answered, today)
		if n > 0 {
			want[user] = n
		}
		// The streak the bot shows right after an answer counts the same way.
		if one, err := svc.MoodStreak(ctx, guildA, user); err != nil || one != n {
			t.Fatalf("MoodStreak(%s) = %d, %v; want %d", user, one, err, n)
		}
	}
	got, err := svc.MemberStreaks(ctx, guildA)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) < 10 {
		t.Fatalf("test data too thin: %v", want)
	}
	t.Logf("streaks: %v", got)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("streaks:\n got %v\nwant %v", got, want)
	}
}
