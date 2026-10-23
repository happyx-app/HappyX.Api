package launch

import (
	"testing"
	"time"
)

var quick = Config{Rounds: 3, Intro: time.Second, Round: 30 * time.Second, Summary: time.Second, Final: time.Second, TaskLife: 20 * time.Second, Seed: 42}

const tick = 50 * time.Millisecond

func run(g *Game, d time.Duration) {
	for ; d > 0; d -= tick {
		g.Step(tick)
	}
}

func newGame(t *testing.T, ids ...string) *Game {
	t.Helper()
	if ids == nil {
		ids = []string{"a", "b", "c"}
	}
	g, err := New(ids, quick)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// workersOf returns the two workers of a three-player game.
func workersOf(g *Game) (string, string) {
	var w []string
	for _, id := range g.Players() {
		if id != g.Manager() {
			w = append(w, id)
		}
	}
	return w[0], w[1]
}

func TestOfficeIsConnected(t *testing.T) {
	for y, row := range layout {
		if len(row) != Cols {
			t.Fatalf("row %d is %d wide", y, len(row))
		}
	}
	for _, s := range spawns {
		if !walkable(s[0], s[1]) {
			t.Fatalf("spawn %v is not floor", s)
		}
	}
	from := spawns[0]
	for _, s := range append(append([]Spot{}, stations...), tables...) {
		if walkable(s.X, s.Y) || path(from[0], from[1], s.X, s.Y) == nil {
			t.Fatalf("can't reach %s at %d,%d", s.ID, s.X, s.Y)
		}
	}
	for _, s := range spawns[1:] {
		if path(from[0], from[1], s[0], s[1]) == nil {
			t.Fatalf("spawn %v is cut off", s)
		}
	}
}

func TestNeedsThreeToTenPlayers(t *testing.T) {
	if _, err := New([]string{"a", "b"}, quick); err != ErrPlayers {
		t.Fatalf("2 players: %v", err)
	}
	ids := make([]string, 11)
	for i := range ids {
		ids[i] = string(rune('a' + i))
	}
	if _, err := New(ids, quick); err != ErrPlayers {
		t.Fatalf("11 players: %v", err)
	}
}

func TestRoundsRotateTheManager(t *testing.T) {
	g := newGame(t)
	if g.Phase() != Intro || len(g.view("a").Tasks)+len(g.view("b").Tasks)+len(g.view("c").Tasks) != FirstTasks {
		t.Fatalf("start: %s", g.Phase())
	}
	managers := map[string]bool{}
	for r := range quick.Rounds {
		if g.Phase() != Intro {
			t.Fatalf("round %d starts in %s", r+1, g.Phase())
		}
		managers[g.Manager()] = true
		m := g.players[g.Manager()]
		if m.tx != seat[0] || m.ty != seat[1] {
			t.Fatal("manager not seated")
		}
		run(g, quick.Intro)
		if g.Phase() != Playing {
			t.Fatalf("after the intro: %s", g.Phase())
		}
		run(g, quick.Round)
		if r < quick.Rounds-1 {
			if g.Phase() != Summary {
				t.Fatalf("after round %d: %s", r+1, g.Phase())
			}
			run(g, quick.Summary)
		}
	}
	if g.Phase() != Final || len(managers) != 3 {
		t.Fatalf("phase %s, managers %v", g.Phase(), managers)
	}
	run(g, quick.Final)
	if !g.Done() {
		t.Fatal("game not over")
	}
}

func TestDeliverATask(t *testing.T) {
	g := newGame(t)
	m := g.Manager()
	w, other := workersOf(g)
	task := g.tasks[0]
	// Workers can't give orders, and nobody walks during the intro.
	g.Apply(w, Input{Order: &Order{Worker: w, Task: task.ID}})
	if task.Worker != "" {
		t.Fatal("a worker gave an order")
	}
	g.Apply(m, Input{Order: &Order{Worker: w, Task: task.ID}})
	if v := g.Views([]string{w})[w]; v.Order == nil || v.Order.ID != task.ID || len(v.Events) == 0 || v.Events[len(v.Events)-1].Type != "order" {
		t.Fatalf("worker's view: %+v", v)
	}
	s := stationOf(task.Item)
	g.Apply(w, Input{Goto: &[2]int{s.X, s.Y}})
	run(g, quick.Intro)
	run(g, 6*time.Second)
	if g.players[w].carry != task.Item {
		t.Fatalf("didn't pick up the %s at %d,%d (at %d,%d)", task.Item, s.X, s.Y, g.players[w].tx, g.players[w].ty)
	}
	if v := g.view(w); v.Order.Target != [2]int{tableOf(task.To).X, tableOf(task.To).Y} {
		t.Fatalf("arrow still points to %v", v.Order.Target)
	}
	tb := tableOf(task.To)
	g.Apply(w, Input{Goto: &[2]int{tb.X, tb.Y}})
	run(g, 6*time.Second)
	if g.delivered[0] != 1 || g.task(task.ID) != nil || g.players[w].task != 0 {
		t.Fatalf("delivered %v", g.delivered)
	}
	evs := g.Views([]string{other})[other].Events
	if len(evs) == 0 || evs[len(evs)-1].Type != "delivered" {
		t.Fatalf("other worker didn't hear of it: %+v", evs)
	}
}

func TestWorkersOnlySeeWhatIsNear(t *testing.T) {
	g := newGame(t)
	m := g.Manager()
	w, other := workersOf(g)
	g.place(g.players[w], [2]int{2, 7})
	g.place(g.players[other], [2]int{21, 7})
	v := g.view(w)
	if v.Role != "worker" || len(v.Players) != 1 || v.Players[0].ID != w || v.Tasks != nil {
		t.Fatalf("far worker visible or tasks leaked: %+v", v)
	}
	g.place(g.players[other], [2]int{4, 7})
	if v := g.view(w); len(v.Players) != 2 {
		t.Fatalf("near worker hidden: %+v", v.Players)
	}
	if v := g.view(m); v.Role != "manager" || len(v.Players) != 3 || len(v.Tasks) != FirstTasks {
		t.Fatalf("manager view: %+v", v)
	}
	if v := g.view("someone"); v.Role != "spectator" || len(v.Players) != 3 {
		t.Fatalf("spectator view: %+v", v)
	}
}

func TestOneOrderPerWorker(t *testing.T) {
	g := newGame(t)
	m := g.Manager()
	w, other := workersOf(g)
	a, b := g.tasks[0], g.tasks[1]
	g.Apply(m, Input{Order: &Order{Worker: w, Task: a.ID}})
	g.Apply(m, Input{Order: &Order{Worker: w, Task: b.ID}})
	if a.Worker != "" || b.Worker != w || g.players[w].task != b.ID {
		t.Fatal("new order didn't replace the old one")
	}
	g.Apply(m, Input{Order: &Order{Worker: other, Task: b.ID}})
	if b.Worker != other || g.players[w].task != 0 {
		t.Fatal("task not moved to the other worker")
	}
	g.Apply(m, Input{Order: &Order{Worker: other}})
	if b.Worker != "" || g.players[other].task != 0 {
		t.Fatal("order not taken back")
	}
	// The manager can't order themselves around.
	g.Apply(m, Input{Order: &Order{Worker: m, Task: a.ID}})
	if a.Worker != "" {
		t.Fatal("manager took a task")
	}
}

func TestTasksExpireAndArrive(t *testing.T) {
	g := newGame(t)
	m := g.Manager()
	w, other := workersOf(g)
	first := g.tasks[0]
	g.Apply(m, Input{Order: &Order{Worker: w, Task: first.ID}})
	g.Views(g.Players())
	run(g, quick.Intro+quick.TaskLife)
	if g.task(first.ID) != nil || g.players[w].task != 0 || g.expired[0] < 1 {
		t.Fatal("task didn't expire")
	}
	vs := g.Views([]string{m, w, other})
	heard := func(id string) bool {
		for _, e := range vs[id].Events {
			if e.Type == "expired" && e.Task == first.ID {
				return true
			}
		}
		return false
	}
	if !heard(m) || !heard(w) || heard(other) {
		t.Fatal("expiry told to the wrong people")
	}
	if len(g.tasks) == 0 {
		t.Fatal("no new tasks came in")
	}
}

func TestWalkingIsOnTheGrid(t *testing.T) {
	g := newGame(t)
	w, _ := workersOf(g)
	p := g.players[w]
	g.place(p, [2]int{2, 7})
	run(g, quick.Intro)
	// Up from the corridor's west end is a wall: the worker stays put.
	g.Apply(w, Input{Move: &[2]int{0, -1}})
	run(g, time.Second)
	if p.tx != 2 || p.ty != 5 {
		t.Fatalf("at %d,%d", p.tx, p.ty)
	}
	// Releasing the keys finishes the step underway and stops on a tile.
	g.Apply(w, Input{Move: &[2]int{1, 0}})
	g.Step(tick)
	g.Apply(w, Input{Move: &[2]int{0, 0}})
	run(g, time.Second)
	if p.tx != 3 || p.x != 3.5 || p.next != nil {
		t.Fatalf("stopped at %v,%v", p.x, p.y)
	}
	// The manager stays in their seat whatever they press.
	m := g.players[g.Manager()]
	g.Apply(m.id, Input{Move: &[2]int{-1, 0}})
	run(g, time.Second)
	if m.tx != seat[0] {
		t.Fatal("manager walked")
	}
}

func TestLeavingHandsOverAndEnds(t *testing.T) {
	g := newGame(t, "a", "b", "c", "d")
	m := g.Manager()
	// A quick reconnect keeps the seat.
	g.Leave(m)
	run(g, Grace/2)
	g.Rejoin(m)
	if g.Manager() != m {
		t.Fatal("manager lost the seat on a reconnect")
	}
	g.Leave(m)
	run(g, Grace)
	if g.Manager() == m || g.Manager() == "" {
		t.Fatal("no new manager")
	}
	nm := g.players[g.Manager()]
	if nm.tx != seat[0] || nm.ty != seat[1] {
		t.Fatal("new manager not seated")
	}
	if v := g.view(m); v.Role != "spectator" {
		t.Fatalf("away player is %s", v.Role)
	}
	g.Rejoin(m)
	if v := g.view(m); v.Role != "worker" {
		t.Fatalf("back as %s", v.Role)
	}
	for _, id := range g.Players() {
		if id != g.Manager() {
			g.Leave(id)
		}
	}
	run(g, Grace)
	if g.Phase() != Final {
		t.Fatalf("one player left, phase %s", g.Phase())
	}
}
