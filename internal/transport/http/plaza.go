package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/moixblau/HappyX.Api/internal/domain"
	"github.com/moixblau/HappyX.Api/internal/plaza"
)

// --- The square, over a WebSocket ---

const (
	plazaPing         = 25 * time.Second // below nginx's 60 s read timeout
	plazaWriteTimeout = 5 * time.Second
	plazaReadLimit    = 1 << 10
	plazaMaxPerSecond = 30 // a held key sends far less; more is a broken or hostile client
)

// Close codes the web understands, beside the standard ones.
var plazaCloseCodes = map[string]websocket.StatusCode{
	plaza.ReasonReplaced: 4000,
	plaza.ReasonSlow:     4001,
	plaza.ReasonShutdown: websocket.StatusGoingAway,
}

// webPlaza puts the viewer in their server's square. Only members who can play the games get
// in; everyone else gets the same 404 as for a server they can't see.
func (s *Server) webPlaza(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	g, err := s.svc.PlazaGuild(r.Context(), v, r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// The server's timeouts are meant for plain requests; a square stays open for as long as
	// the tab does.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	// Accept refuses an Origin other than the Host, so other sites can't open a square with
	// the viewer's cookie.
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept has already answered
	}
	ws.SetReadLimit(plazaReadLimit)
	c := s.web.Plaza.Join(g.DiscordID, plaza.Profile{ID: v.DiscordID, Name: v.Username, Avatar: v.Avatar})
	defer c.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		defer cancel()
		readPlaza(ctx, ws, c)
	}()
	ping := time.NewTicker(plazaPing)
	defer ping.Stop()
	for {
		select {
		case msg := <-c.Out():
			wctx, done := context.WithTimeout(ctx, plazaWriteTimeout)
			err := ws.Write(wctx, websocket.MessageText, msg)
			done()
			if err != nil {
				ws.CloseNow()
				return
			}
		case <-ping.C:
			pctx, done := context.WithTimeout(ctx, plazaWriteTimeout)
			err := ws.Ping(pctx)
			done()
			if err != nil {
				ws.CloseNow()
				return
			}
		case <-c.Done():
			reason := c.Reason()
			code, ok := plazaCloseCodes[reason]
			if !ok {
				code = websocket.StatusNormalClosure
			}
			ws.Close(code, reason)
			return
		case <-ctx.Done():
			ws.CloseNow()
			return
		}
	}
}

// plazaMessage is what the web sends: {"t":"move","dx":1,"dy":0}, {"t":"goto","x":10,"y":20}
// or {"t":"emote","e":"wave"}.
type plazaMessage struct {
	Type  string  `json:"t"`
	DX    int     `json:"dx"`
	DY    int     `json:"dy"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Emote string  `json:"e"`
}

func readPlaza(ctx context.Context, ws *websocket.Conn, c *plaza.Conn) {
	window, n := time.Now(), 0
	for {
		typ, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		if now := time.Now(); now.Sub(window) >= time.Second {
			window, n = now, 0
		}
		if n++; n > plazaMaxPerSecond || typ != websocket.MessageText {
			continue
		}
		var m plazaMessage
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if in, ok := m.input(); ok {
			c.Send(in)
		}
	}
}

func (m plazaMessage) input() (plaza.Input, bool) {
	switch m.Type {
	case "move":
		return plaza.Input{Move: &[2]int{m.DX, m.DY}}, true
	case "goto":
		return plaza.Input{Goto: &[2]float64{m.X, m.Y}}, true
	case "emote":
		return plaza.Input{Emote: plaza.Emote(m.Emote)}, true
	}
	return plaza.Input{}, false
}
