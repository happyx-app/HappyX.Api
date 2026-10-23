// Package launch is «El gran lanzamiento», the office game played from a server's square.
//
// One player is the manager: they sit in the boss's office, see the whole floor and every
// pending task, but can't move. The others are workers: they walk the office but only see
// what is close to them and the order the manager gave them. Tasks are delivered only when
// the two talk to each other. The manager changes every round.
//
// Like the square, the game is authoritative: it lives in memory, moves everyone on a grid
// and decides what each player is allowed to see. Nothing about a game is stored yet.
package launch

// Tile is how many world units a grid cell spans, so the office fills the same 960x600 world
// as the square.
const Tile = 40

// The office floor, one character per tile:
//
//	#  wall          d  desk
//	.  floor         T  a meeting room's table, where deliveries go
//	P  printer       A  storage shelves      C  coffee machine
//	M  the manager's seat
var layout = []string{
	"########################",
	"#......#.......#.......#",
	"#..T...#...C...#...T...#",
	"#......#.......#.......#",
	"###..#####...#####..####",
	"#......................#",
	"#..dd...dd....dd...dd..#",
	"#......................#",
	"#..dd...dd....dd...dd..#",
	"#......................#",
	"###..#####...#####..####",
	"#......#.......#.......#",
	"#..P...#...A...#..T..M.#",
	"#......#.......#.......#",
	"########################",
}

// Cols and Rows are the office's size in tiles.
var (
	Cols = len(layout[0])
	Rows = len(layout)
)

// Items workers carry, and where they pick them up.
const (
	Document = "doc"
	Box      = "box"
	Coffee   = "coffee"
)

// Rooms a task can be delivered to.
const (
	BlueRoom  = "blue"
	GreenRoom = "green"
	BossRoom  = "boss"
)

// Room is a labelled area of the office, in tiles. The web draws its floor and name.
type Room struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	W  int    `json:"w"`
	H  int    `json:"h"`
}

// Spot is a tile something happens next to: a station that hands out an item, or the table a
// room's deliveries go to.
type Spot struct {
	ID   string `json:"id"` // the item for a station, the room for a table
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Room string `json:"room"`
}

// Office is the map the web draws. The layout is the same for every game.
type Office struct {
	Tile     int      `json:"tile"`
	Cols     int      `json:"cols"`
	Rows     int      `json:"rows"`
	Tiles    []string `json:"tiles"`
	Rooms    []Room   `json:"rooms"`
	Stations []Spot   `json:"stations"`
	Tables   []Spot   `json:"tables"`
	Seat     [2]int   `json:"seat"`
	Vision   float64  `json:"vision"` // how far a worker sees, in tiles
}

var rooms = []Room{
	{BlueRoom, 1, 1, 6, 3},
	{"cafe", 8, 1, 7, 3},
	{GreenRoom, 16, 1, 7, 3},
	{"print", 1, 11, 6, 3},
	{"store", 8, 11, 7, 3},
	{BossRoom, 16, 11, 7, 3},
}

var (
	stations = []Spot{{Document, 3, 12, "print"}, {Box, 11, 12, "store"}, {Coffee, 11, 2, "cafe"}}
	tables   = []Spot{{BlueRoom, 3, 2, BlueRoom}, {GreenRoom, 19, 2, GreenRoom}, {BossRoom, 18, 12, BossRoom}}
	seat     = [2]int{21, 12}
)

// Items and Destinations are what tasks are made of.
var (
	Items        = []string{Document, Box, Coffee}
	Destinations = []string{BlueRoom, GreenRoom, BossRoom}
)

// Map returns the office for the web.
func Map() Office {
	return Office{Tile, Cols, Rows, layout, rooms, stations, tables, seat, Vision}
}

// walkable reports whether a worker can stand on the tile.
func walkable(x, y int) bool {
	return x >= 0 && y >= 0 && x < Cols && y < Rows && layout[y][x] == '.'
}

func stationOf(item string) Spot {
	for _, s := range stations {
		if s.ID == item {
			return s
		}
	}
	return Spot{}
}

func tableOf(room string) Spot {
	for _, t := range tables {
		if t.ID == room {
			return t
		}
	}
	return Spot{}
}

// spawns are where workers start each round: the open-plan floor between the desks.
var spawns = [][2]int{
	{6, 7}, {11, 7}, {17, 7}, {12, 5}, {6, 9}, {17, 9}, {11, 9}, {2, 7}, {21, 7}, {12, 7},
}

// path is the shortest walk from (x, y) to a tile next to (gx, gy), or to (gx, gy)
// itself when it is floor, without the starting tile. It is nil when there is nowhere to go.
func path(x, y, gx, gy int) [][2]int {
	goal := func(cx, cy int) bool {
		if walkable(gx, gy) {
			return cx == gx && cy == gy
		}
		return abs(cx-gx)+abs(cy-gy) == 1
	}
	if goal(x, y) {
		return nil
	}
	type node struct{ x, y int }
	from := map[node]node{{x, y}: {x, y}}
	queue := []node{{x, y}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			m := node{n.x + d[0], n.y + d[1]}
			if _, seen := from[m]; seen || !walkable(m.x, m.y) {
				continue
			}
			from[m] = n
			if goal(m.x, m.y) {
				var out [][2]int
				for c := m; c != (node{x, y}); c = from[c] {
					out = append([][2]int{{c.x, c.y}}, out...)
				}
				return out
			}
			queue = append(queue, m)
		}
	}
	return nil
}

// near reports whether the tile (x, y) is next to (tx, ty), diagonals included.
func near(x, y, tx, ty int) bool { return abs(x-tx) <= 1 && abs(y-ty) <= 1 }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
