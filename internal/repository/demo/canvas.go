package demo

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// The demo's team canvas: a few drawings in the margins of a 1600×1000 screen, made up the same
// way every time. Like the rest of the demo it can be looked at, not changed.

type sketch struct {
	author string // a person's name
	color  int
	ago    time.Duration
	lines  [][][2]float64 // px, x from the screen's centre and y from the top
}

func demoSketches() []sketch {
	seed := uint32(7)
	rnd := func() float64 { seed = seed*1103515245 + 12345; return float64(seed>>8&0xffff) / 0xffff }
	wobble := func(ps [][2]float64) [][2]float64 {
		ox, oy := 0.0, 0.0
		out := make([][2]float64, len(ps))
		for i, p := range ps {
			ox, oy = ox*0.7+(rnd()-0.5)*1.6, oy*0.7+(rnd()-0.5)*1.6
			out[i] = [2]float64{p[0] + ox, p[1] + oy}
		}
		return out
	}
	curve := func(n int, f func(t float64) [2]float64) [][2]float64 {
		ps := make([][2]float64, n+1)
		for i := range ps {
			ps[i] = f(float64(i) / float64(n))
		}
		return wobble(ps)
	}
	circle := func(cx, cy, rx, ry, from, to float64) [][2]float64 {
		return curve(60, func(t float64) [2]float64 {
			a := 2 * math.Pi * (from + (to-from)*t)
			return [2]float64{cx + rx*math.Cos(a), cy + ry*math.Sin(a)}
		})
	}
	line := func(x1, y1, x2, y2 float64) [][2]float64 {
		return curve(12, func(t float64) [2]float64 { return [2]float64{x1 + (x2-x1)*t, y1 + (y2-y1)*t} })
	}
	star := func(cx, cy, r float64) [][2]float64 {
		ps := make([][2]float64, 11)
		for i := range ps {
			a, rr := -math.Pi/2+float64(i)*math.Pi/5, r
			if i%2 == 1 {
				rr = r * 0.45
			}
			ps[i] = [2]float64{cx + rr*math.Cos(a), cy + rr*math.Sin(a)}
		}
		return wobble(ps)
	}
	const violet, yellow, pink, blue, green = 0, 1, 2, 3, 4
	h := time.Hour
	sun := [][][2]float64{circle(-700, 190, 34, 34, 0, 1.05)}
	for k := 0; k < 8; k++ {
		a := float64(k)/8*2*math.Pi + 0.2
		sun = append(sun, line(-700+48*math.Cos(a), 190+48*math.Sin(a), -700+72*math.Cos(a), 190+72*math.Sin(a)))
	}
	heart := curve(80, func(t float64) [2]float64 {
		a := t * 2 * math.Pi
		return [2]float64{-640 + 38.4*math.Pow(math.Sin(a), 3), 430 - 2.4*(13*math.Cos(a)-5*math.Cos(2*a)-2*math.Cos(3*a)-math.Cos(4*a))}
	})
	cat := [][][2]float64{
		circle(-690, 640, 46, 40, 0, 1.03),
		wobble([][2]float64{{-728, 618}, {-722, 572}, {-700, 602}}),
		wobble([][2]float64{{-680, 602}, {-658, 572}, {-652, 618}}),
		circle(-707, 634, 4, 4, 0, 1.05), circle(-673, 634, 4, 4, 0, 1.05),
		curve(10, func(t float64) [2]float64 { return [2]float64{-700 + 20*t, 656 + 6*math.Sin(math.Pi*t)} }),
		line(-740, 650, -712, 652), line(-668, 652, -640, 650),
	}
	plant := [][][2]float64{
		curve(20, func(t float64) [2]float64 { return [2]float64{-640 + 6*math.Sin(t*3), 900 - 130*t} }),
		curve(20, func(t float64) [2]float64 { return [2]float64{-640 + 46*t, 840 - 30*math.Sin(math.Pi*t) - 10*t} }),
		curve(20, func(t float64) [2]float64 { return [2]float64{-640 - 40*t, 810 - 26*math.Sin(math.Pi*t) - 8*t} }),
	}
	pot := [][][2]float64{wobble([][2]float64{{-675, 900}, {-605, 900}, {-615, 960}, {-665, 960}, {-675, 900}})}
	cup := [][][2]float64{
		wobble([][2]float64{{615, 520}, {625, 610}, {700, 610}, {710, 520}, {615, 520}}),
		circle(718, 560, 20, 22, -0.25, 0.25),
	}
	for _, x := range []float64{640, 662, 684} {
		cup = append(cup, curve(14, func(t float64) [2]float64 { return [2]float64{x + 6*math.Sin(t*2*math.Pi), 500 - 50*t} }))
	}
	face := [][][2]float64{
		circle(680, 1000, 44, 44, 0, 1.05), circle(665, 988, 3, 3, 0, 1.05), circle(697, 988, 3, 3, 0, 1.05),
		curve(12, func(t float64) [2]float64 { return [2]float64{660 + 40*t, 1012 + 12*math.Sin(math.Pi*t)} }),
	}
	return []sketch{
		{"pau", yellow, 6 * 24 * h, sun},
		{"nuria", pink, 3 * 24 * h, [][][2]float64{heart}},
		{"elena", violet, 2 * 24 * h, cat},
		{"javi", green, 26 * h, plant},
		{"javi", blue, 26 * h, pot},
		{"marta", yellow, 4 * 24 * h, [][][2]float64{star(640, 170, 34), star(730, 250, 20), star(620, 300, 14)}},
		{"dani", blue, 30 * h, cup},
		{"sofia", violet, 5 * 24 * h, face},
	}
}

// demoCanvas turns the sketches into strokes. A drawing's strokes share its anchor, the centre of
// its points as a fraction of the screen, and its points are px from there.
func demoCanvas(now time.Time) []domain.CanvasStroke {
	ids := map[string]string{}
	for _, p := range people {
		ids[p.name] = p.id
	}
	sketches := demoSketches()
	key := func(sk sketch) string { return fmt.Sprint(sk.author, sk.ago) }
	// The centre of each drawing, over all its sketches.
	type sum struct{ x, y, n float64 }
	centres := map[string]*sum{}
	drawings := map[string]string{}
	for _, sk := range sketches {
		k := key(sk)
		if centres[k] == nil {
			centres[k], drawings[k] = &sum{}, fmt.Sprintf("demo-%d", len(drawings)+1)
		}
		for _, l := range sk.lines {
			for _, p := range l {
				c := centres[k]
				c.x, c.y, c.n = c.x+p[0], c.y+p[1], c.n+1
			}
		}
	}
	var out []domain.CanvasStroke
	var id int64
	for _, sk := range sketches {
		k := key(sk)
		cx, cy := centres[k].x/centres[k].n, centres[k].y/centres[k].n
		for _, l := range sk.lines {
			id++
			st := domain.CanvasStroke{
				ID: id, GuildDiscordID: GuildID, AuthorDiscordID: ids[sk.author], AuthorUsername: sk.author,
				DrawingID: drawings[k], Color: sk.color, AnchorX: (800 + cx) / 1600, AnchorY: cy / 1000,
				CreatedAt: now.Add(-sk.ago),
			}
			for _, p := range l {
				st.Points = append(st.Points, domain.CanvasPoint{X: int16(math.Round(p[0] - cx)), Y: int16(math.Round(p[1] - cy))})
			}
			out = append(out, st)
		}
	}
	return out
}

type canvas struct{ s *Store }

func (c canvas) List(_ context.Context, guildID string) ([]domain.CanvasStroke, int64, error) {
	if guildID != GuildID {
		return nil, 0, domain.ErrNotFound
	}
	return demoCanvas(c.s.now()), 1, nil
}

func (canvas) Version(_ context.Context, guildID string) (int64, error) {
	if guildID != GuildID {
		return 0, domain.ErrNotFound
	}
	return 1, nil
}

func (c canvas) Count(ctx context.Context, guildID string) (int, error) {
	ss, _, err := c.List(ctx, guildID)
	return len(ss), err
}

func (canvas) CountByAuthorSince(context.Context, string, string, time.Time) (int, error) {
	return 0, nil
}
func (canvas) Add(context.Context, domain.CanvasStroke) (domain.CanvasStroke, int64, error) {
	return domain.CanvasStroke{}, 0, ErrReadOnly
}
func (canvas) DeleteDrawing(context.Context, string, string, string) (int, int64, error) {
	return 0, 0, ErrReadOnly
}
func (canvas) Clear(context.Context, string) (int64, error) { return 0, ErrReadOnly }
