package launch

import (
	"errors"
	"math"
	"math/rand/v2"
	"time"
)

const (
	MinPlayers = 3 // a manager and two workers
	MaxPlayers = 10
	// Speed is how many tiles a worker walks per second.
	Speed = 4.0
	// Vision is how far a worker sees, in tiles.
	Vision = 3.5
	// FirstTasks is how many tasks are waiting when a round starts.
	FirstTasks = 3
	// Grace is how long a player who dropped out keeps their place, so a reconnect doesn't
	// cost the team their order.
	Grace = 10 * time.Second
)

// Phase is where the game is.
type Phase string

const (
	Intro   Phase = "intro"   // the new manager is announced; nobody moves yet
	Playing Phase = "playing" // tasks come in and are delivered
	Summary Phase = "summary" // the round's tally, before the next one
	Final   Phase = "final"   // the game's tally
	Over    Phase = "over"    // the square takes over again
)

// Config sets how long each part lasts. Tests shorten it.
type Config struct {
	Rounds   int
	Intro    time.Duration
	Round    time.Duration
	Summary  time.Duration
	Final    time.Duration
	TaskLife time.Duration
	Seed     uint64
}

// Default is the game as designed: 3 rounds of 4 minutes, about 15 minutes in all.
var Default = Config{
	Rounds:   3,
	Intro:    10 * time.Second,
	Round:    4 * time.Minute,
	Summary:  8 * time.Second,
	Final:    30 * time.Second,
	TaskLife: 75 * time.Second,
}

var ErrPlayers = errors.New("the game needs 3 to 10 players")

// Task is something the client asked for: take an item to a room.
type Task struct {
	ID     int     `json:"id"`
	Item   string  `json:"item"`
	To     string  `json:"to"`
	Left   float64 `json:"left"` // seconds until the client gives up on it
	Worker string  `json:"worker,omitempty"`

	left time.Duration
}

type player struct {
	id     string
	tx, ty int     // the tile they are on, or leaving
	x, y   float64 // where they are, in tiles; tile centres are at .5
	next   *[2]int // the tile they are walking onto
	path   [][2]int
	dx, dy int
	carry  string
	task   int
	gone   bool          // their connection dropped
	away   time.Duration // for how long
	out    bool          // away past the grace: their order went back to the queue
}

// Input is what a player pressed.
type Input struct {
	// Move holds a direction with the keyboard; 0, 0 stops at the next tile.
	Move *[2]int
	// Goto walks to a tile, for taps and clicks. A station or table means the tile beside it.
	Goto *[2]int
	// Order is the manager giving a task to a worker. Task 0 takes their order back.
	Order *Order
}

type Order struct {
	Worker string
	Task   int
}

// Event is something players should notice, beyond what the next view shows.
type Event struct {
	Type    string `json:"t"` // round, order, picked, delivered, expired, task, manager
	Round   int    `json:"round,omitempty"`
	Manager string `json:"manager,omitempty"`
	Task    int    `json:"task,omitempty"`
	Worker  string `json:"worker,omitempty"`
	Item    string `json:"item,omitempty"`
	To      string `json:"to,omitempty"`

	only []string // who hears it; nobody listed means everyone
}

// Game is one game of «El gran lanzamiento». It is not safe for concurrent use; the square's
// hub serialises access.
type Game struct {
	cfg     Config
	rng     *rand.Rand
	order   []string // the manager rotation, shuffled once
	players map[string]*player

	phase     Phase
	round     int // from 0
	left      time.Duration
	manager   string
	tasks     []*Task
	nextID    int
	spawnIn   time.Duration
	delivered []int // per round
	expired   []int
	events    []Event
}

// New starts a game with the given players, in the order they are listed.
func New(ids []string, cfg Config) (*Game, error) {
	if len(ids) < MinPlayers || len(ids) > MaxPlayers {
		return nil, ErrPlayers
	}
	seed := cfg.Seed
	if seed == 0 {
		seed = rand.Uint64()
	}
	g := &Game{cfg: cfg, rng: rand.New(rand.NewPCG(seed, seed>>1)), players: map[string]*player{}}
	g.order = append([]string(nil), ids...)
	g.rng.Shuffle(len(g.order), func(i, j int) { g.order[i], g.order[j] = g.order[j], g.order[i] })
	for _, id := range ids {
		g.players[id] = &player{id: id}
	}
	g.startRound()
	return g, nil
}

// Players lists who is in the game, in the manager rotation's order.
func (g *Game) Players() []string { return append([]string(nil), g.order...) }

// Has reports whether id plays in this game, even if they are away right now.
func (g *Game) Has(id string) bool { _, ok := g.players[id]; return ok }

func (g *Game) Phase() Phase { return g.phase }

func (g *Game) Manager() string { return g.manager }

// Done reports whether the game is over and the square can take over again.
func (g *Game) Done() bool { return g.phase == Over }

// Stop ends the game at once.
func (g *Game) Stop() { g.phase = Over }

// Step moves the game dt forward.
func (g *Game) Step(dt time.Duration) {
	if g.phase == Over {
		return
	}
	g.left -= dt
	for _, id := range g.order {
		if p := g.players[id]; p.gone && !p.out {
			if p.away += dt; p.away >= Grace {
				g.release(p)
				if g.phase == Over || g.phase == Final {
					return
				}
			}
		}
	}
	if g.phase == Playing {
		g.tickTasks(dt)
		for _, id := range g.order {
			if p := g.players[id]; !p.gone && id != g.manager {
				g.walk(p, dt.Seconds()*Speed)
			}
		}
	}
	if g.left <= 0 {
		g.advance()
	}
}

func (g *Game) advance() {
	switch g.phase {
	case Intro:
		g.phase, g.left = Playing, g.cfg.Round
	case Playing:
		g.tasks = nil
		for _, p := range g.players {
			p.task, p.carry = 0, ""
		}
		if g.round+1 < g.cfg.Rounds {
			g.phase, g.left = Summary, g.cfg.Summary
		} else {
			g.phase, g.left = Final, g.cfg.Final
		}
	case Summary:
		g.round++
		g.startRound()
	case Final:
		g.phase = Over
	}
}

// startRound seats the next manager, puts the workers back on the floor and opens the first
// tasks.
func (g *Game) startRound() {
	g.manager = ""
	for i := range g.order {
		if id := g.order[(g.round+i)%len(g.order)]; !g.players[id].out {
			g.manager = id
			break
		}
	}
	g.delivered = append(g.delivered, 0)
	g.expired = append(g.expired, 0)
	g.tasks = nil
	n := 0
	for _, id := range g.order {
		p := g.players[id]
		p.task, p.carry, p.path, p.next, p.dx, p.dy = 0, "", nil, nil, 0, 0
		if id == g.manager {
			g.place(p, seat)
		} else {
			g.place(p, spawns[n%len(spawns)])
			n++
		}
	}
	for range FirstTasks {
		g.addTask()
	}
	g.spawnIn = g.interval()
	g.phase, g.left = Intro, g.cfg.Intro
	g.emit(Event{Type: "round", Round: g.round + 1, Manager: g.manager})
}

func (g *Game) place(p *player, t [2]int) {
	p.tx, p.ty = t[0], t[1]
	p.x, p.y = float64(t[0])+0.5, float64(t[1])+0.5
}

func (g *Game) workers() int {
	n := 0
	for id, p := range g.players {
		if !p.out && id != g.manager {
			n++
		}
	}
	return n
}

// interval is how often a new task comes in: more workers, more work.
func (g *Game) interval() time.Duration {
	w := max(1, g.workers())
	return min(20*time.Second, max(6*time.Second, 40*time.Second/time.Duration(w)))
}

// addTask opens a task that isn't already waiting.
func (g *Game) addTask() {
	var item, to string
	for range 10 {
		item, to = Items[g.rng.IntN(len(Items))], Destinations[g.rng.IntN(len(Destinations))]
		if !g.waiting(item, to) {
			break
		}
	}
	g.nextID++
	g.tasks = append(g.tasks, &Task{ID: g.nextID, Item: item, To: to, left: g.cfg.TaskLife})
}

func (g *Game) waiting(item, to string) bool {
	for _, t := range g.tasks {
		if t.Item == item && t.To == to {
			return true
		}
	}
	return false
}

func (g *Game) tickTasks(dt time.Duration) {
	kept := g.tasks[:0]
	for _, t := range g.tasks {
		if t.left -= dt; t.left > 0 {
			kept = append(kept, t)
			continue
		}
		g.expired[g.round]++
		if p := g.players[t.Worker]; p != nil {
			p.task = 0
		}
		g.emit(Event{Type: "expired", Task: t.ID, Item: t.Item, To: t.To, Worker: t.Worker, only: []string{g.manager, t.Worker}})
	}
	g.tasks = kept
	if g.spawnIn -= dt; g.spawnIn <= 0 {
		g.spawnIn = g.interval()
		if len(g.tasks) < g.workers()+3 {
			g.addTask()
			g.emit(Event{Type: "task", Task: g.nextID, only: []string{g.manager}})
		}
	}
}

func (g *Game) task(id int) *Task {
	for _, t := range g.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// walk moves a worker dist tiles along their path or held direction, one tile at a time.
func (g *Game) walk(p *player, dist float64) {
	for dist > 0 {
		if p.next == nil {
			n, ok := p.nextTile()
			if !ok {
				return
			}
			p.next = &n
		}
		cx, cy := float64(p.next[0])+0.5, float64(p.next[1])+0.5
		d := math.Hypot(cx-p.x, cy-p.y)
		if d > dist {
			p.x += (cx - p.x) / d * dist
			p.y += (cy - p.y) / d * dist
			return
		}
		p.x, p.y, p.tx, p.ty = cx, cy, p.next[0], p.next[1]
		p.next = nil
		dist -= d
		g.arrive(p)
	}
}

func (p *player) nextTile() ([2]int, bool) {
	if len(p.path) > 0 {
		n := p.path[0]
		p.path = p.path[1:]
		return n, true
	}
	// Held diagonals go sideways first, then up or down, so walking into a wall slides.
	if p.dx != 0 && walkable(p.tx+p.dx, p.ty) {
		return [2]int{p.tx + p.dx, p.ty}, true
	}
	if p.dy != 0 && walkable(p.tx, p.ty+p.dy) {
		return [2]int{p.tx, p.ty + p.dy}, true
	}
	return [2]int{}, false
}

// arrive picks up or delivers when a worker reaches the right place for their order.
func (g *Game) arrive(p *player) {
	t := g.task(p.task)
	if t == nil {
		return
	}
	if p.carry != t.Item {
		if s := stationOf(t.Item); near(p.tx, p.ty, s.X, s.Y) {
			p.carry = t.Item
			g.emit(Event{Type: "picked", Task: t.ID, Worker: p.id, Item: t.Item, only: []string{p.id, g.manager}})
		}
		return
	}
	if tb := tableOf(t.To); near(p.tx, p.ty, tb.X, tb.Y) {
		g.removeTask(t.ID)
		p.task, p.carry = 0, ""
		g.delivered[g.round]++
		g.emit(Event{Type: "delivered", Task: t.ID, Worker: p.id, Item: t.Item, To: t.To})
	}
}

func (g *Game) removeTask(id int) {
	for i, t := range g.tasks {
		if t.ID == id {
			g.tasks = append(g.tasks[:i], g.tasks[i+1:]...)
			return
		}
	}
}

// Apply handles what a player pressed.
func (g *Game) Apply(id string, in Input) {
	p := g.players[id]
	if p == nil || p.gone || (g.phase != Intro && g.phase != Playing) {
		return
	}
	switch {
	case in.Order != nil:
		if id == g.manager {
			g.give(*in.Order)
		}
	case id == g.manager:
		// The manager runs the floor from their seat.
	case in.Move != nil:
		p.dx, p.dy = sign(in.Move[0]), sign(in.Move[1])
		p.path = nil
	case in.Goto != nil:
		from := [2]int{p.tx, p.ty}
		if p.next != nil {
			from = *p.next
		}
		p.path = path(from[0], from[1], in.Goto[0], in.Goto[1])
		p.dx, p.dy = 0, 0
	}
}

// give hands a task to a worker. Each worker has one order at a time, so a new one replaces
// the last, which goes back to the queue.
func (g *Game) give(o Order) {
	w := g.players[o.Worker]
	if w == nil || w.gone || o.Worker == g.manager {
		return
	}
	if o.Task == 0 {
		if t := g.task(w.task); t != nil {
			t.Worker = ""
		}
		w.task = 0
		return
	}
	t := g.task(o.Task)
	if t == nil {
		return
	}
	if old := g.task(w.task); old != nil {
		old.Worker = ""
	}
	if other := g.players[t.Worker]; other != nil {
		other.task = 0
	}
	t.Worker, w.task = w.id, t.ID
	g.emit(Event{Type: "order", Task: t.ID, Worker: w.id, Item: t.Item, To: t.To, only: []string{w.id}})
	if w.next == nil {
		g.arrive(w) // already standing at the right place
	}
}

// Leave marks a player whose connection dropped. Their avatar waits where it is; after the
// grace, a worker's order goes back to the queue and a manager hands the seat on.
func (g *Game) Leave(id string) {
	p := g.players[id]
	if p == nil || p.gone {
		return
	}
	p.gone, p.away = true, 0
	p.path, p.dx, p.dy = nil, 0, 0
}

func (g *Game) release(p *player) {
	p.out = true
	if t := g.task(p.task); t != nil {
		t.Worker = ""
	}
	p.task, p.carry, p.next = 0, "", nil
	if g.present() < 2 {
		if g.phase != Final && g.phase != Over {
			g.phase, g.left = Final, g.cfg.Final
		}
		return
	}
	if p.id == g.manager && (g.phase == Intro || g.phase == Playing) {
		for i := range g.order {
			q := g.players[g.order[(g.round+1+i)%len(g.order)]]
			if !q.gone {
				if t := g.task(q.task); t != nil {
					t.Worker = ""
				}
				q.task, q.carry, q.path, q.next, q.dx, q.dy = 0, "", nil, nil, 0, 0
				g.manager = q.id
				g.place(q, seat)
				break
			}
		}
		g.emit(Event{Type: "manager", Manager: g.manager})
	}
}

// present counts the players who haven't been away past the grace.
func (g *Game) present() int {
	n := 0
	for _, p := range g.players {
		if !p.out {
			n++
		}
	}
	return n
}

// Rejoin brings back a player who left. Within the grace they carry on where they were;
// after it they come back as a worker on the open floor.
func (g *Game) Rejoin(id string) {
	p := g.players[id]
	if p == nil || !p.gone {
		return
	}
	if p.out && id != g.manager {
		g.place(p, spawns[0])
	}
	p.gone, p.out, p.away = false, false, 0
}

func (g *Game) emit(e Event) { g.events = append(g.events, e) }

// PlayerView is an avatar as one player sees it, in world units like the square.
type PlayerView struct {
	ID    string  `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Carry string  `json:"carry,omitempty"`
	Task  int     `json:"task,omitempty"`
	Away  bool    `json:"away,omitempty"` // their connection dropped
}

// OrderView is a worker's current order, with the tile the arrow points to.
type OrderView struct {
	Task
	Target [2]int `json:"target"`
}

// View is everything one player may know right now. Workers only get the avatars near them
// and their own order: filtering here, not in the browser, is what keeps it fair.
type View struct {
	Type      string       `json:"t"`
	Phase     Phase        `json:"phase"`
	Round     int          `json:"round"`
	Rounds    int          `json:"rounds"`
	Left      float64      `json:"left"`
	Manager   string       `json:"manager"`
	Role      string       `json:"role"` // manager, worker or spectator
	Players   []PlayerView `json:"players"`
	Tasks     []Task       `json:"tasks,omitempty"`
	Order     *OrderView   `json:"order,omitempty"`
	Delivered []int        `json:"delivered"`
	Expired   []int        `json:"expired"`
	Events    []Event      `json:"ev,omitempty"`
}

// Views builds what each viewer sees and clears the events they were sent. Viewers who don't
// play watch as spectators, who see everything.
func (g *Game) Views(viewers []string) map[string]View {
	out := make(map[string]View, len(viewers))
	for _, id := range viewers {
		out[id] = g.view(id)
	}
	g.events = nil
	return out
}

func (g *Game) view(id string) View {
	v := View{
		Type: "game", Phase: g.phase, Round: g.round + 1, Rounds: g.cfg.Rounds,
		Left: math.Max(0, math.Round(g.left.Seconds()*10)/10), Manager: g.manager,
		Delivered: g.delivered, Expired: g.expired, Players: []PlayerView{},
	}
	me := g.players[id]
	switch {
	case me == nil || me.gone:
		v.Role = "spectator"
	case id == g.manager:
		v.Role = "manager"
	default:
		v.Role = "worker"
	}
	for _, pid := range g.order {
		p := g.players[pid]
		if p.out {
			continue
		}
		if v.Role == "worker" && p != me && math.Hypot(p.x-me.x, p.y-me.y) > Vision {
			continue
		}
		v.Players = append(v.Players, PlayerView{p.id, round1(p.x * Tile), round1(p.y * Tile), p.carry, p.task, p.gone})
	}
	if v.Role == "worker" {
		if t := g.task(me.task); t != nil {
			o := &OrderView{Task: g.taskView(t)}
			if me.carry == t.Item {
				tb := tableOf(t.To)
				o.Target = [2]int{tb.X, tb.Y}
			} else {
				s := stationOf(t.Item)
				o.Target = [2]int{s.X, s.Y}
			}
			v.Order = o
		}
	} else {
		v.Tasks = make([]Task, 0, len(g.tasks))
		for _, t := range g.tasks {
			v.Tasks = append(v.Tasks, g.taskView(t))
		}
	}
	for _, e := range g.events {
		if e.only == nil || contains(e.only, id) || v.Role == "spectator" {
			v.Events = append(v.Events, e)
		}
	}
	return v
}

func (g *Game) taskView(t *Task) Task {
	c := *t
	c.Left = math.Ceil(t.left.Seconds())
	return c
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
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

func round1(v float64) float64 { return math.Round(v*10) / 10 }
