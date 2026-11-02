package domain

import (
	"errors"
	"regexp"
	"time"
)

// The team canvas: members draw on the background of the server's dashboard pages. A stroke is
// one line, from pressing to lifting the pen; the strokes of one drawing share a DrawingID and
// are erased together.

var (
	// ErrCanvasOff means the guild hasn't turned the canvas on.
	ErrCanvasOff = errors.New("the canvas is turned off in this guild")
	// ErrCanvasFull means the guild's canvas holds MaxCanvasStrokes and something must be erased.
	ErrCanvasFull = errors.New("the canvas is full: erase something first")
)

const (
	// CanvasColors is how many pen colours there are; the dashboard decides what they look like.
	CanvasColors = 5
	// MaxCanvasPoints is the most points one stroke may have.
	MaxCanvasPoints = 500
	// MaxCanvasStrokes is the most strokes one guild's canvas may hold.
	MaxCanvasStrokes = 3000
	// MaxCanvasStrokesPerMinute is how many strokes one person may add in a minute.
	MaxCanvasStrokesPerMinute = 60
	// MaxCanvasOffset bounds a point's distance from its anchor, in px.
	MaxCanvasOffset = 4000
)

// CanvasPoint is a point in px from its stroke's anchor.
type CanvasPoint struct{ X, Y int16 }

type CanvasStroke struct {
	ID              int64
	GuildDiscordID  string
	AuthorDiscordID string
	AuthorUsername  string
	DrawingID       string
	Color           int
	// AnchorX and AnchorY place the drawing on the screen, as a fraction of its width and
	// height, so a drawing in the margin stays in the margin on any screen.
	AnchorX, AnchorY float64
	Points           []CanvasPoint
	CreatedAt        time.Time
}

var drawingIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,36}$`)

// ValidDrawingID reports whether id can name a drawing.
func ValidDrawingID(id string) bool { return drawingIDPattern.MatchString(id) }

func (s CanvasStroke) Validate() error {
	if !ValidDrawingID(s.DrawingID) {
		return invalid("drawing_id must be 1 to 36 letters, digits, - or _")
	}
	if s.Color < 0 || s.Color >= CanvasColors {
		return invalid("color must be between 0 and %d", CanvasColors-1)
	}
	if !(s.AnchorX >= 0 && s.AnchorX <= 1 && s.AnchorY >= 0 && s.AnchorY <= 1) {
		return invalid("anchor must be between 0 and 1")
	}
	if len(s.Points) < 2 || len(s.Points) > MaxCanvasPoints {
		return invalid("a stroke must have 2 to %d points", MaxCanvasPoints)
	}
	for _, p := range s.Points {
		if p.X < -MaxCanvasOffset || p.X > MaxCanvasOffset || p.Y < -MaxCanvasOffset || p.Y > MaxCanvasOffset {
			return invalid("points must be within %d px of the anchor", MaxCanvasOffset)
		}
	}
	return nil
}

// EncodeCanvasPoints packs points as big-endian int16 pairs, 4 bytes a point, as they are stored.
func EncodeCanvasPoints(ps []CanvasPoint) []byte {
	out := make([]byte, 0, 4*len(ps))
	for _, p := range ps {
		out = append(out, byte(uint16(p.X)>>8), byte(uint16(p.X)), byte(uint16(p.Y)>>8), byte(uint16(p.Y)))
	}
	return out
}

// DecodeCanvasPoints is the inverse of EncodeCanvasPoints.
func DecodeCanvasPoints(b []byte) []CanvasPoint {
	out := make([]CanvasPoint, 0, len(b)/4)
	for i := 0; i+3 < len(b); i += 4 {
		out = append(out, CanvasPoint{int16(uint16(b[i])<<8 | uint16(b[i+1])), int16(uint16(b[i+2])<<8 | uint16(b[i+3]))})
	}
	return out
}
