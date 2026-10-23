// Package plaza is a server's square: a small shared map where the members who have the web
// open walk around as their Discord avatar and greet each other in real time. It is the lobby
// the team games will start from.
//
// The server is authoritative: clients only send what their player pressed, and every tick the
// room moves the avatars and tells everyone where they are. Nothing here is stored; a restart
// empties the squares.
package plaza

import (
	"errors"
	"hash/fnv"
	"math"
	"time"
)

// The world is measured in units; the web scales it to fit the screen.
const (
	Width  = 960
	Height = 600
	// Radius is an avatar's size for collisions.
	Radius = 18
	// Speed is how far an avatar walks per second.
	Speed = 180
	// MaxPlayers is how many avatars share one square. When a square is full, newcomers open
	// another one of the same server.
	MaxPlayers = 30
	// HighFiveRange is how close two avatars must be to high-five.
	HighFiveRange = 80
	// HighFiveWindow is how long a raised hand waits for someone to answer it.
	HighFiveWindow = 3 * time.Second
	// EmoteCooldown keeps a player from flooding the square with emotes.
	EmoteCooldown = 800 * time.Millisecond
	// Colors is the number of avatar border colours the web knows.
	Colors = 8
)

// Emote is a quick greeting. There is no free text, so there is nothing to moderate.
type Emote string

const (
	Wave     Emote = "wave"
	Clap     Emote = "clap"
	Laugh    Emote = "laugh"
	Heart    Emote = "heart"
	Coffee   Emote = "coffee"
	HighFive Emote = "highfive"
)

func (e Emote) Valid() bool {
	switch e {
	case Wave, Clap, Laugh, Heart, Coffee, HighFive:
		return true
	}
	return false
}

// Obstacle is something avatars walk around. Kind tells the web how to draw it.
type Obstacle struct {
	Kind string  `json:"kind"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

// Round obstacles (the fountain and the trees) are circles as wide as their box, so avatars
// walk around them the way they are drawn.
func (o Obstacle) Round() bool { return o.Kind == "fountain" || o.Kind == "tree" }

// Obstacles is the square's layout: a fountain in the middle, benches around it and trees in
// the corners.
var Obstacles = []Obstacle{
	{"fountain", 420, 240, 120, 120},
	{"bench", 230, 140, 100, 28},
	{"bench", 630, 140, 100, 28},
	{"bench", 230, 432, 100, 28},
	{"bench", 630, 432, 100, 28},
	{"tree", 70, 60, 64, 64},
	{"tree", 826, 60, 64, 64},
	{"tree", 70, 476, 64, 64},
	{"tree", 826, 476, 64, 64},
}

// spawns are free spots around the fountain where avatars appear, in turn.
var spawns = [][2]float64{
	{480, 200}, {580, 300}, {480, 400}, {380, 300},
	{560, 210}, {560, 390}, {400, 390}, {400, 210},
}

// Profile is who a player is, from their Discord login.
type Profile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
}

// Player is an avatar in the square.
type Player struct {
	Profile
	Color int     `json:"color"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`

	dx, dy    float64 // held direction, each -1, 0 or 1
	target    *[2]float64
	lastEmote time.Time
	handUp    time.Time // when they last asked for a high five
}

// Input is what a player pressed.
type Input struct {
	// Move holds a direction with the keyboard: DX and DY are -1, 0 or 1. 0, 0 stops.
	Move *[2]int
	// Goto walks to a point, for taps and clicks.
	Goto  *[2]float64
	Emote Emote
}

// Event is something everyone in the square must hear about.
type Event struct {
	Type string `json:"t"`
	// join
	Player *Player `json:"player,omitempty"`
	// leave, emote
	ID    string `json:"id,omitempty"`
	Emote Emote  `json:"e,omitempty"`
	// high_five
	A string `json:"a,omitempty"`
	B string `json:"b,omitempty"`
}

var ErrFull = errors.New("the square is full")

// Room is one square's state. It is not safe for concurrent use; the hub serialises access.
type Room struct {
	players map[string]*Player
	order   []string // join order, so snapshots are stable
	spawned int
}

func NewRoom() *Room { return &Room{players: map[string]*Player{}} }

func (r *Room) Len() int { return len(r.players) }

func (r *Room) Has(id string) bool { _, ok := r.players[id]; return ok }

// Players returns the avatars in join order.
func (r *Room) Players() []*Player {
	out := make([]*Player, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.players[id])
	}
	return out
}

// Join places a new avatar at the next free spawn point.
func (r *Room) Join(p Profile) (*Player, error) {
	if len(r.players) >= MaxPlayers {
		return nil, ErrFull
	}
	pl := &Player{Profile: p, Color: colorOf(p.ID)}
	for i := range spawns {
		s := spawns[(r.spawned+i)%len(spawns)]
		if !r.crowded(s[0], s[1]) {
			pl.X, pl.Y = s[0], s[1]
			break
		}
	}
	if pl.X == 0 { // every spawn is taken: share the first one
		pl.X, pl.Y = spawns[r.spawned%len(spawns)][0], spawns[r.spawned%len(spawns)][1]
	}
	r.spawned++
	r.players[p.ID] = pl
	r.order = append(r.order, p.ID)
	return pl, nil
}

func (r *Room) crowded(x, y float64) bool {
	for _, p := range r.players {
		if math.Hypot(p.X-x, p.Y-y) < 2*Radius {
			return true
		}
	}
	return false
}

func (r *Room) Leave(id string) {
	if _, ok := r.players[id]; !ok {
		return
	}
	delete(r.players, id)
	for i, o := range r.order {
		if o == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// Apply handles a player's input at now and returns the events it causes.
func (r *Room) Apply(id string, in Input, now time.Time) []Event {
	p, ok := r.players[id]
	if !ok {
		return nil
	}
	switch {
	case in.Move != nil:
		p.dx, p.dy = float64(sign(in.Move[0])), float64(sign(in.Move[1]))
		p.target = nil
	case in.Goto != nil:
		t := [2]float64{clamp(in.Goto[0], Radius, Width-Radius), clamp(in.Goto[1], Radius, Height-Radius)}
		p.target = &t
		p.dx, p.dy = 0, 0
	case in.Emote != "":
		if !in.Emote.Valid() || now.Sub(p.lastEmote) < EmoteCooldown {
			return nil
		}
		p.lastEmote = now
		evs := []Event{{Type: "emote", ID: id, Emote: in.Emote}}
		if in.Emote == HighFive {
			if o := r.waitingHand(p, now); o != nil {
				o.handUp, p.handUp = time.Time{}, time.Time{}
				evs = append(evs, Event{Type: "high_five", A: o.ID, B: p.ID})
			} else {
				p.handUp = now
			}
		}
		return evs
	}
	return nil
}

// waitingHand is the closest player in range who raised their hand recently.
func (r *Room) waitingHand(p *Player, now time.Time) *Player {
	var best *Player
	bestD := math.Inf(1)
	for _, id := range r.order {
		o := r.players[id]
		if o == p || o.handUp.IsZero() || now.Sub(o.handUp) > HighFiveWindow {
			continue
		}
		if d := math.Hypot(o.X-p.X, o.Y-p.Y); d <= HighFiveRange && d < bestD {
			best, bestD = o, d
		}
	}
	return best
}

// Step moves every avatar dt forward and reports whether any of them moved.
func (r *Room) Step(dt time.Duration) bool {
	moved := false
	step := Speed * dt.Seconds()
	for _, id := range r.order {
		p := r.players[id]
		dx, dy := p.dx, p.dy
		if p.target != nil {
			dx, dy = p.target[0]-p.X, p.target[1]-p.Y
			d := math.Hypot(dx, dy)
			if d <= step {
				moved = r.moveTo(p, p.target[0], p.target[1]) || moved
				p.target = nil
				continue
			}
			dx, dy = dx/d, dy/d
		} else if dx != 0 && dy != 0 {
			dx, dy = dx/math.Sqrt2, dy/math.Sqrt2
		}
		if dx == 0 && dy == 0 {
			continue
		}
		x0, y0 := p.X, p.Y
		r.walk(p, dx, dy, step)
		if p.X == x0 && p.Y == y0 {
			p.target = nil // walked into something: stop instead of pushing forever
		} else {
			moved = true
		}
	}
	return moved
}

// detours are the turns tried, closest first, when the way ahead is blocked.
var detours = []float64{math.Pi / 6, -math.Pi / 6, math.Pi / 3, -math.Pi / 3}

// walk takes a full step in direction (dx, dy). If something is in the way it turns as little
// as it can to go around it, so avatars round the fountain instead of crawling along it.
func (r *Room) walk(p *Player, dx, dy, step float64) {
	x, y := p.X+dx*step, p.Y+dy*step
	if inside(x, y) && !blocked(x, y) {
		p.X, p.Y = x, y
		return
	}
	for _, a := range detours {
		sin, cos := math.Sincos(a)
		x, y := p.X+(dx*cos-dy*sin)*step, p.Y+(dx*sin+dy*cos)*step
		if inside(x, y) && !blocked(x, y) {
			p.X, p.Y = x, y
			return
		}
	}
	r.moveTo(p, x, y)
}

func inside(x, y float64) bool {
	return x >= Radius && x <= Width-Radius && y >= Radius && y <= Height-Radius
}

// moveTo moves one axis at a time, so an avatar slides along an obstacle instead of sticking.
func (r *Room) moveTo(p *Player, x, y float64) bool {
	x0, y0 := p.X, p.Y
	x, y = clamp(x, Radius, Width-Radius), clamp(y, Radius, Height-Radius)
	if !blocked(x, p.Y) {
		p.X = x
	}
	if !blocked(p.X, y) {
		p.Y = y
	}
	return p.X != x0 || p.Y != y0
}

func blocked(x, y float64) bool {
	for _, o := range Obstacles {
		if o.Round() {
			if math.Hypot(x-(o.X+o.W/2), y-(o.Y+o.H/2)) < Radius+o.W/2 {
				return true
			}
			continue
		}
		cx, cy := clamp(x, o.X, o.X+o.W), clamp(y, o.Y, o.Y+o.H)
		if math.Hypot(x-cx, y-cy) < Radius {
			return true
		}
	}
	return false
}

func colorOf(id string) int {
	h := fnv.New32a()
	h.Write([]byte(id))
	return int(h.Sum32() % Colors)
}

func sign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
