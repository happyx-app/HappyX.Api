package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// WithCanvas gives the Service the team canvas' storage.
func (s *Service) WithCanvas(c CanvasRepository) *Service {
	s.canvas = c
	return s
}

// CanvasView is a guild's canvas as one viewer sees it.
type CanvasView struct {
	Enabled bool
	Version int64
	// Strokes is empty while the canvas is off, except for moderators.
	Strokes []domain.CanvasStroke
	// CanModerate: the viewer may change the guild's settings, so they see who drew what and may
	// erase any drawing.
	CanModerate bool
}

// canvasGuild lets in whoever can open one of the guild's dashboard pages: those who may see
// its reports, and its game's players. Anyone else gets ErrNotFound, as for a guild they can't
// see. moderator is true for those who may change the guild's settings.
func (s *Service) canvasGuild(ctx context.Context, v domain.Viewer, guildID string) (g domain.Guild, moderator bool, err error) {
	if s.canvas == nil || v.DiscordID == "" {
		return domain.Guild{}, false, domain.ErrNotFound
	}
	if g, err = s.guilds.Get(ctx, guildID); err != nil {
		return domain.Guild{}, false, err
	}
	if !g.Active {
		return domain.Guild{}, false, domain.ErrNotFound
	}
	level, err := s.accessOf(ctx, g, v.DiscordID, v.ManagesInDiscord(g.DiscordID))
	if err != nil {
		return domain.Guild{}, false, err
	}
	if level >= domain.AccessView {
		return g, level >= domain.AccessManage, nil
	}
	if !g.Settings.GamesEnabled {
		return domain.Guild{}, false, domain.ErrNotFound
	}
	switch err := s.player(ctx, g, v.DiscordID); {
	case errors.Is(err, domain.ErrNotAMember):
		return domain.Guild{}, false, domain.ErrNotFound
	case err != nil:
		return domain.Guild{}, false, err
	}
	return g, false, nil
}

// Canvas returns the guild's canvas for the viewer.
func (s *Service) Canvas(ctx context.Context, v domain.Viewer, guildID string) (CanvasView, error) {
	g, moderator, err := s.canvasGuild(ctx, v, guildID)
	if err != nil {
		return CanvasView{}, err
	}
	out := CanvasView{Enabled: g.Settings.CanvasEnabled, CanModerate: moderator, Strokes: []domain.CanvasStroke{}}
	if !out.Enabled && !moderator {
		// The version still goes in, so the page's ETag matches CanvasVersion and it gets 304s.
		out.Version, err = s.canvas.Version(ctx, g.DiscordID)
		return out, err
	}
	if out.Strokes, out.Version, err = s.canvas.List(ctx, g.DiscordID); err != nil {
		return CanvasView{}, err
	}
	return out, nil
}

// CanvasVersion tells the dashboard whether the canvas changed since it last read it, without
// reading the strokes.
func (s *Service) CanvasVersion(ctx context.Context, v domain.Viewer, guildID string) (enabled, moderator bool, version int64, err error) {
	g, moderator, err := s.canvasGuild(ctx, v, guildID)
	if err != nil {
		return false, false, 0, err
	}
	if version, err = s.canvas.Version(ctx, g.DiscordID); err != nil {
		return false, false, 0, err
	}
	return g.Settings.CanvasEnabled, moderator, version, nil
}

// AddCanvasStroke adds a stroke by the viewer to the guild's canvas.
func (s *Service) AddCanvasStroke(ctx context.Context, v domain.Viewer, guildID string, st domain.CanvasStroke) (domain.CanvasStroke, int64, error) {
	g, _, err := s.canvasGuild(ctx, v, guildID)
	if err != nil {
		return domain.CanvasStroke{}, 0, err
	}
	if !g.Settings.CanvasEnabled {
		return domain.CanvasStroke{}, 0, domain.ErrCanvasOff
	}
	st.GuildDiscordID, st.AuthorDiscordID = g.DiscordID, v.DiscordID
	if err := st.Validate(); err != nil {
		return domain.CanvasStroke{}, 0, err
	}
	out, version, err := s.canvas.Add(ctx, st, s.now().Add(-time.Minute))
	if errors.Is(err, domain.ErrNotFound) {
		// Someone who sees the reports without being in the server (an owner the bot never
		// synced, say) has no member row to sign the stroke with.
		return domain.CanvasStroke{}, 0, domain.ErrNotAMember
	}
	return out, version, err
}

// DeleteCanvasDrawing erases one drawing: any drawing for moderators, only their own for the
// rest. It works while the canvas is off too, so nothing stays stuck there.
func (s *Service) DeleteCanvasDrawing(ctx context.Context, v domain.Viewer, guildID, drawingID string) (int64, error) {
	g, moderator, err := s.canvasGuild(ctx, v, guildID)
	if err != nil {
		return 0, err
	}
	if !domain.ValidDrawingID(drawingID) {
		return 0, domain.ErrNotFound
	}
	author := v.DiscordID
	if moderator {
		author = ""
	}
	n, version, err := s.canvas.DeleteDrawing(ctx, g.DiscordID, drawingID, author)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, domain.ErrNotFound
	}
	return version, nil
}

// ClearCanvas erases the whole canvas. Only those who may change the guild's settings may do it.
func (s *Service) ClearCanvas(ctx context.Context, v domain.Viewer, guildID string) (int64, error) {
	if s.canvas == nil {
		return 0, domain.ErrNotFound
	}
	g, err := s.ViewerGuild(ctx, v, guildID, domain.AccessManage)
	if err != nil {
		return 0, err
	}
	return s.canvas.Clear(ctx, g.DiscordID)
}
