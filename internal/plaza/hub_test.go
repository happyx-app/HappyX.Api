package plaza

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// next waits for the next message of type typ, skipping others.
func next(t *testing.T, c *Conn, typ string) map[string]any {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case b := <-c.Out():
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			if m["t"] == typ {
				return m
			}
		case <-timeout:
			t.Fatalf("no %q message", typ)
		}
	}
}

func TestHubWelcomeJoinLeave(t *testing.T) {
	h := NewHub()
	a := h.Join("g1", Profile{ID: "a", Name: "Ana"})
	w := next(t, a, "welcome")
	if w["you"] != "a" || len(w["players"].([]any)) != 1 || w["world"].(map[string]any)["w"].(float64) != Width {
		t.Fatalf("welcome: %v", w)
	}
	b := h.Join("g1", Profile{ID: "b", Name: "Bea"})
	if j := next(t, a, "join"); j["player"].(map[string]any)["name"] != "Bea" {
		t.Fatalf("join: %v", j)
	}
	if w := next(t, b, "welcome"); len(w["players"].([]any)) != 2 {
		t.Fatalf("b's welcome: %v", w)
	}
	b.Close()
	if l := next(t, a, "leave"); l["id"] != "b" {
		t.Fatalf("leave: %v", l)
	}
	select {
	case <-b.Done():
	default:
		t.Fatal("closed connection not done")
	}
	a.Close()
	if h.Count("g1") != 0 || len(h.squares) != 0 {
		t.Fatal("empty square kept")
	}
}

func TestHubMovesAndBroadcastsState(t *testing.T) {
	h := NewHub()
	a := h.Join("g1", Profile{ID: "a"})
	b := h.Join("g1", Profile{ID: "b"})
	defer a.Close()
	defer b.Close()
	a.Send(Input{Move: &[2]int{0, -1}})
	st := next(t, b, "state")
	ps := st["p"].([]any)
	if len(ps) != 2 || ps[0].(map[string]any)["id"] != "a" {
		t.Fatalf("state: %v", st)
	}
	b.Send(Input{Emote: Wave})
	if e := next(t, a, "emote"); e["id"] != "b" || e["e"] != "wave" {
		t.Fatalf("emote: %v", e)
	}
}

func TestHubKeepsServersApart(t *testing.T) {
	h := NewHub()
	a := h.Join("g1", Profile{ID: "a"})
	b := h.Join("g2", Profile{ID: "b"})
	defer a.Close()
	defer b.Close()
	if w := next(t, b, "welcome"); len(w["players"].([]any)) != 1 {
		t.Fatalf("g2 sees g1: %v", w)
	}
	b.Send(Input{Emote: Wave})
	time.Sleep(50 * time.Millisecond)
	for len(a.Out()) > 0 {
		if m := string(<-a.Out()); strings.Contains(m, `"b"`) {
			t.Fatalf("g1 heard g2: %s", m)
		}
	}
}

func TestHubOneAvatarPerPerson(t *testing.T) {
	h := NewHub()
	old := h.Join("g1", Profile{ID: "a"})
	other := h.Join("g1", Profile{ID: "b"})
	defer other.Close()
	nw := h.Join("g1", Profile{ID: "a"})
	defer nw.Close()
	<-old.Done()
	if old.Reason() != ReasonReplaced || h.Count("g1") != 2 {
		t.Fatalf("reason %q, count %d", old.Reason(), h.Count("g1"))
	}
	// The old tab's input no longer moves the avatar.
	old.Send(Input{Emote: Wave})
	old.Close()
	if h.Count("g1") != 2 {
		t.Fatal("closing the replaced tab removed the new one")
	}
}

func TestHubOpensParallelSquares(t *testing.T) {
	h := NewHub()
	var cs []*Conn
	for i := range MaxPlayers + 1 {
		cs = append(cs, h.Join("g1", Profile{ID: string(rune('A' + i))}))
	}
	if n := len(h.squares["g1"]); n != 2 || h.Count("g1") != MaxPlayers+1 {
		t.Fatalf("%d squares, %d people", n, h.Count("g1"))
	}
	if w := next(t, cs[MaxPlayers], "welcome"); len(w["players"].([]any)) != 1 {
		t.Fatal("31st person not alone in the new square")
	}
	h.Close()
	for _, c := range cs {
		<-c.Done()
	}
	if len(h.squares) != 0 {
		t.Fatal("squares left after Close")
	}
}

func TestHubDropsSlowConnections(t *testing.T) {
	h := NewHub()
	slow := h.Join("g1", Profile{ID: "slow"})
	fast := h.Join("g1", Profile{ID: "fast"})
	defer fast.Close()
	// fast reads everything; slow reads nothing until its buffer overflows.
	go func() {
		for {
			select {
			case <-fast.Out():
			case <-fast.Done():
				return
			}
		}
	}()
	for i := 0; i < 2*outBuffer; i++ {
		h.mu.Lock()
		h.broadcast(fast.sq, Event{Type: "emote", ID: "fast", Emote: Wave})
		h.mu.Unlock()
	}
	select {
	case <-slow.Done():
	case <-time.After(time.Second):
		t.Fatal("slow connection kept")
	}
	if slow.Reason() != ReasonSlow || h.Count("g1") != 1 {
		t.Fatalf("reason %q", slow.Reason())
	}
}
