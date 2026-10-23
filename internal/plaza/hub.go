package plaza

import (
	"encoding/json"
	"math"
	"sync"
	"time"
)

// TickRate is how many times per second the squares move and send positions.
const TickRate = 20

// outBuffer is how many messages a connection may fall behind before it is dropped: a few
// seconds of positions.
const outBuffer = 64

// Reasons a connection is closed by the hub.
const (
	ReasonReplaced = "replaced" // the same person opened the square somewhere else
	ReasonSlow     = "slow"     // the connection couldn't keep up
	ReasonShutdown = "shutdown"
)

// Hub holds the squares of every server. Each server has its own squares, so members of one
// server never see another's; a full square opens a parallel one of the same server.
type Hub struct {
	mu      sync.Mutex
	squares map[string][]*square // by guild ID
	tick    time.Duration
	now     func() time.Time
}

type square struct {
	guild string
	room  *Room
	conns map[string]*Conn
	stop  chan struct{}
}

// Conn is one player's connection to a square. Messages for them arrive on Out, already
// encoded as JSON; Done closes when the hub drops them.
type Conn struct {
	hub    *Hub
	sq     *square
	id     string
	out    chan []byte
	done   chan struct{}
	reason string
}

func NewHub() *Hub {
	return &Hub{squares: map[string][]*square{}, tick: time.Second / TickRate, now: time.Now}
}

// world is what a newcomer needs to draw the square.
type world struct {
	W         int        `json:"w"`
	H         int        `json:"h"`
	Radius    int        `json:"radius"`
	Obstacles []Obstacle `json:"obstacles"`
}

// Join puts a member of guild in one of its squares. If they were already in, the older
// connection is closed: one avatar per person.
func (h *Hub) Join(guild string, p Profile) *Conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sq := range h.squares[guild] {
		if old, ok := sq.conns[p.ID]; ok {
			h.leave(old, ReasonReplaced)
		}
	}
	var sq *square
	for _, s := range h.squares[guild] {
		if s.room.Len() < MaxPlayers {
			sq = s
			break
		}
	}
	if sq == nil {
		sq = &square{guild: guild, room: NewRoom(), conns: map[string]*Conn{}, stop: make(chan struct{})}
		h.squares[guild] = append(h.squares[guild], sq)
		go h.run(sq)
	}
	pl, _ := sq.room.Join(p) // the square was picked with room to spare
	c := &Conn{hub: h, sq: sq, id: p.ID, out: make(chan []byte, outBuffer), done: make(chan struct{})}
	h.broadcast(sq, Event{Type: "join", Player: pl})
	sq.conns[p.ID] = c
	c.out <- encode(struct {
		Type    string    `json:"t"`
		You     string    `json:"you"`
		World   world     `json:"world"`
		Players []*Player `json:"players"`
	}{"welcome", p.ID, world{Width, Height, Radius, Obstacles}, sq.room.Players()})
	return c
}

// Out delivers the messages for this player.
func (c *Conn) Out() <-chan []byte { return c.out }

// Done closes when the hub drops the connection; Reason then says why.
func (c *Conn) Done() <-chan struct{} { return c.done }

func (c *Conn) Reason() string {
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	return c.reason
}

// Send applies what the player pressed.
func (c *Conn) Send(in Input) {
	h := c.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.sq.conns[c.id] != c {
		return
	}
	for _, ev := range c.sq.room.Apply(c.id, in, h.now()) {
		h.broadcast(c.sq, ev)
	}
}

// Close takes the player out of the square.
func (c *Conn) Close() {
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	c.hub.leave(c, "")
}

// Close drops every connection, for a graceful shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sqs := range h.squares {
		for _, sq := range sqs {
			for _, c := range sq.conns {
				h.leave(c, ReasonShutdown)
			}
		}
	}
}

// Count returns how many people are in the server's squares.
func (h *Hub) Count(guild string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, sq := range h.squares[guild] {
		n += sq.room.Len()
	}
	return n
}

func (h *Hub) run(sq *square) {
	t := time.NewTicker(h.tick)
	defer t.Stop()
	for {
		select {
		case <-sq.stop:
			return
		case <-t.C:
			h.mu.Lock()
			if sq.room.Step(h.tick) {
				h.broadcastState(sq)
			}
			h.mu.Unlock()
		}
	}
}

type position struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

func (h *Hub) broadcastState(sq *square) {
	ps := make([]position, 0, sq.room.Len())
	for _, p := range sq.room.Players() {
		ps = append(ps, position{p.ID, round1(p.X), round1(p.Y)})
	}
	h.send(sq, encode(struct {
		Type    string     `json:"t"`
		Players []position `json:"p"`
	}{"state", ps}))
}

// leave must be called with h.mu held.
func (h *Hub) leave(c *Conn, reason string) {
	sq := c.sq
	if sq.conns[c.id] != c {
		return
	}
	delete(sq.conns, c.id)
	sq.room.Leave(c.id)
	c.reason = reason
	close(c.done)
	if len(sq.conns) == 0 {
		close(sq.stop)
		list := h.squares[sq.guild]
		for i, s := range list {
			if s == sq {
				list = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(list) == 0 {
			delete(h.squares, sq.guild)
		} else {
			h.squares[sq.guild] = list
		}
		return
	}
	h.broadcast(sq, Event{Type: "leave", ID: c.id})
}

func (h *Hub) broadcast(sq *square, ev Event) { h.send(sq, encode(ev)) }

// send must be called with h.mu held. A connection whose buffer is full is dropped rather
// than holding up the square.
func (h *Hub) send(sq *square, msg []byte) {
	var slow []*Conn
	for _, c := range sq.conns {
		select {
		case c.out <- msg:
		default:
			slow = append(slow, c)
		}
	}
	for _, c := range slow {
		h.leave(c, ReasonSlow)
	}
}

func encode(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
