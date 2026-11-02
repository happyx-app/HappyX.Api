package domain

import "testing"

func TestCanvasPointsRoundTrip(t *testing.T) {
	ps := []CanvasPoint{{0, 0}, {-1, 1}, {MaxCanvasOffset, -MaxCanvasOffset}, {-32768, 32767}}
	b := EncodeCanvasPoints(ps)
	if len(b) != 4*len(ps) {
		t.Fatalf("%d bytes for %d points", len(b), len(ps))
	}
	got := DecodeCanvasPoints(b)
	for i := range ps {
		if got[i] != ps[i] {
			t.Fatalf("point %d: %v, want %v", i, got[i], ps[i])
		}
	}
}

func TestCanvasStrokeValidate(t *testing.T) {
	ok := CanvasStroke{DrawingID: "a-1_B", Color: 4, AnchorX: 0, AnchorY: 1, Points: []CanvasPoint{{0, 0}, {3, 4}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	many := make([]CanvasPoint, MaxCanvasPoints+1)
	for name, s := range map[string]CanvasStroke{
		"empty id":  {Color: 0, Points: ok.Points},
		"long id":   {DrawingID: "0123456789012345678901234567890123456", Points: ok.Points},
		"colour":    {DrawingID: "a", Color: CanvasColors, Points: ok.Points},
		"negative":  {DrawingID: "a", Color: -1, Points: ok.Points},
		"anchor":    {DrawingID: "a", AnchorX: -0.1, Points: ok.Points},
		"one point": {DrawingID: "a", Points: ok.Points[:1]},
		"too many":  {DrawingID: "a", Points: many},
		"far":       {DrawingID: "a", Points: []CanvasPoint{{0, 0}, {MaxCanvasOffset + 1, 0}}},
	} {
		if err := s.Validate(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
