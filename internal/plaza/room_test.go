package plaza

import (
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func joinAt(t *testing.T, r *Room, id string, x, y float64) *Player {
	t.Helper()
	p, err := r.Join(Profile{ID: id, Name: id})
	if err != nil {
		t.Fatal(err)
	}
	p.X, p.Y = x, y
	return p
}

func TestJoinSpawnsApartAndFills(t *testing.T) {
	r := NewRoom()
	a, _ := r.Join(Profile{ID: "a"})
	b, _ := r.Join(Profile{ID: "b"})
	if math.Hypot(a.X-b.X, a.Y-b.Y) < 2*Radius {
		t.Fatalf("spawned on top of each other: %v,%v and %v,%v", a.X, a.Y, b.X, b.Y)
	}
	if blocked(a.X, a.Y) || blocked(b.X, b.Y) {
		t.Fatal("spawned inside an obstacle")
	}
	for i := 2; i < MaxPlayers; i++ {
		if _, err := r.Join(Profile{ID: string(rune('A' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Join(Profile{ID: "late"}); err != ErrFull {
		t.Fatalf("31st player: %v", err)
	}
}

func TestMoveWithKeys(t *testing.T) {
	r := NewRoom()
	p := joinAt(t, r, "a", 300, 80)
	r.Apply("a", Input{Move: &[2]int{1, 0}}, t0)
	if !r.Step(time.Second) || p.X != 300+Speed || p.Y != 80 {
		t.Fatalf("after 1s right: %v,%v", p.X, p.Y)
	}
	// Diagonals are as fast as straight lines.
	r.Apply("a", Input{Move: &[2]int{-5, 7}}, t0)
	x, y := p.X, p.Y
	r.Step(100 * time.Millisecond)
	if d := math.Hypot(p.X-x, p.Y-y); math.Abs(d-Speed*0.1) > 1e-9 {
		t.Fatalf("diagonal step %v", d)
	}
	r.Apply("a", Input{Move: &[2]int{0, 0}}, t0)
	if r.Step(time.Second) {
		t.Fatal("stopped avatar moved")
	}
}

func TestEdgesAndObstacles(t *testing.T) {
	r := NewRoom()
	p := joinAt(t, r, "a", 30, 300)
	r.Apply("a", Input{Move: &[2]int{-1, 0}}, t0)
	r.Step(time.Second)
	if p.X != Radius {
		t.Fatalf("left the map: x=%v", p.X)
	}
	// Walking into the fountain stops at its edge.
	f := Obstacles[0]
	q := joinAt(t, r, "b", f.X-100, f.Y+f.H/2)
	r.Apply("b", Input{Move: &[2]int{1, 0}}, t0)
	for range 40 {
		r.Step(50 * time.Millisecond)
	}
	if q.X > f.X-Radius+1 || blocked(q.X, q.Y) {
		t.Fatalf("walked into the fountain: x=%v", q.X)
	}
	// Going diagonally into it slides along its side.
	r.Apply("b", Input{Move: &[2]int{1, 1}}, t0)
	y := q.Y
	r.Step(100 * time.Millisecond)
	if q.Y <= y {
		t.Fatal("didn't slide along the fountain")
	}
}

func TestGotoStopsAtTarget(t *testing.T) {
	r := NewRoom()
	p := joinAt(t, r, "a", 100, 300)
	r.Apply("a", Input{Goto: &[2]float64{160, 220}}, t0)
	for range 20 {
		r.Step(50 * time.Millisecond)
	}
	if p.X != 160 || p.Y != 220 {
		t.Fatalf("at %v,%v", p.X, p.Y)
	}
	if r.Step(50 * time.Millisecond) {
		t.Fatal("kept walking after arriving")
	}
	// Targets off the map are pulled inside.
	r.Apply("a", Input{Goto: &[2]float64{-500, 220}}, t0)
	for range 40 {
		r.Step(50 * time.Millisecond)
	}
	if p.X != Radius {
		t.Fatalf("x=%v", p.X)
	}
}

func TestEmotes(t *testing.T) {
	r := NewRoom()
	joinAt(t, r, "a", 100, 300)
	if evs := r.Apply("a", Input{Emote: "dance"}, t0); evs != nil {
		t.Fatalf("unknown emote: %v", evs)
	}
	if evs := r.Apply("a", Input{Emote: Wave}, t0); len(evs) != 1 || evs[0].Type != "emote" || evs[0].ID != "a" {
		t.Fatalf("wave: %v", evs)
	}
	if evs := r.Apply("a", Input{Emote: Heart}, t0.Add(EmoteCooldown/2)); evs != nil {
		t.Fatal("cooldown ignored")
	}
	if evs := r.Apply("a", Input{Emote: Heart}, t0.Add(EmoteCooldown)); len(evs) != 1 {
		t.Fatal("emote after the cooldown dropped")
	}
	if evs := r.Apply("ghost", Input{Emote: Wave}, t0); evs != nil {
		t.Fatal("player not in the room emoted")
	}
}

func TestHighFive(t *testing.T) {
	r := NewRoom()
	joinAt(t, r, "a", 100, 300)
	joinAt(t, r, "b", 150, 300)
	far := joinAt(t, r, "c", 400, 80)

	if evs := r.Apply("a", Input{Emote: HighFive}, t0); len(evs) != 1 {
		t.Fatalf("a raises a hand: %v", evs)
	}
	// Someone out of range can't take it.
	if evs := r.Apply("c", Input{Emote: HighFive}, t0.Add(time.Second)); len(evs) != 1 {
		t.Fatalf("far player matched: %v", evs)
	}
	evs := r.Apply("b", Input{Emote: HighFive}, t0.Add(2*time.Second))
	if len(evs) != 2 || evs[1].Type != "high_five" || evs[1].A != "a" || evs[1].B != "b" {
		t.Fatalf("high five: %v", evs)
	}
	// Both hands are down afterwards; a late hand from a waits again.
	if evs := r.Apply("a", Input{Emote: HighFive}, t0.Add(3*time.Second)); len(evs) != 1 {
		t.Fatalf("hands not reset: %v", evs)
	}
	// A hand left up too long expires.
	far.X, far.Y = 100, 340
	if evs := r.Apply("c", Input{Emote: HighFive}, t0.Add(3*time.Second+HighFiveWindow+time.Millisecond)); len(evs) != 1 {
		t.Fatalf("stale hand matched: %v", evs)
	}
}

func TestLeaveKeepsOrder(t *testing.T) {
	r := NewRoom()
	for _, id := range []string{"a", "b", "c"} {
		r.Join(Profile{ID: id})
	}
	r.Leave("b")
	r.Leave("nobody")
	ps := r.Players()
	if len(ps) != 2 || ps[0].ID != "a" || ps[1].ID != "c" || r.Has("b") {
		t.Fatalf("%v", ps)
	}
}

func TestWalksAroundTheFountain(t *testing.T) {
	r := NewRoom()
	p := joinAt(t, r, "a", 480, 400) // right below the fountain
	r.Apply("a", Input{Goto: &[2]float64{530, 200}}, t0)
	for range 60 {
		r.Step(50 * time.Millisecond)
	}
	if p.X != 530 || p.Y != 200 {
		t.Fatalf("stuck at %v,%v", p.X, p.Y)
	}
	// Pushing straight into a wall doesn't slide along it.
	q := joinAt(t, r, "b", Radius, 300)
	r.Apply("b", Input{Move: &[2]int{-1, 0}}, t0)
	if r.Step(time.Second); q.X != Radius || q.Y != 300 {
		t.Fatalf("wall push moved to %v,%v", q.X, q.Y)
	}
}
