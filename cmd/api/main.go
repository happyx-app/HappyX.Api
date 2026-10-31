package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // timezone validation must not depend on the image having zoneinfo

	"github.com/happyx-app/HappyX.Api/internal/config"
	"github.com/happyx-app/HappyX.Api/internal/discord"
	"github.com/happyx-app/HappyX.Api/internal/repository/backups"
	"github.com/happyx-app/HappyX.Api/internal/repository/demo"
	"github.com/happyx-app/HappyX.Api/internal/repository/postgres"
	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

// version is set at build time (-ldflags "-X main.version=1.2.3"); the admin dashboard shows it.
var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	svc := usecase.NewService(
		postgres.NewGuildRepo(pool),
		postgres.NewMemberRepo(pool),
		postgres.NewMoodRepo(pool), postgres.NewDispatchRepo(pool), postgres.NewKudosRepo(pool), postgres.NewGameRepo(pool), postgres.NewAccessRepo(pool),
		postgres.NewPrivacyRepo(pool),
		time.Now,
	)
	api := httpapi.NewServer(svc, cfg.ServiceKeyHashes, log)
	if w := cfg.Web; w != nil {
		api.WithWeb(httpapi.Web{
			Identity:   discord.NewOAuth(w.DiscordClientID, w.DiscordClientSecret, w.RedirectURL()),
			SessionKey: w.SessionSecret,
			PublicURL:  w.PublicURL,
		})
		log.Info("web dashboard login enabled", "redirect_url", w.RedirectURL())
		if w.Demo {
			// The demo has its own Service, on a made-up server kept in memory: it never reads
			// the database.
			services := map[string]*usecase.Service{}
			for _, lang := range demo.Languages {
				services[lang] = demo.NewStore(lang, time.Now).NewService()
			}
			api.WithDemo(httpapi.Demo{Services: services, Viewer: demo.Viewer()})
			log.Info("public demo enabled", "path", "/web/demo/{lang}", "languages", demo.Languages)
		}
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	servers := []*http.Server{srv}
	if a := cfg.Admin; a != nil {
		admin := usecase.NewAdmin(postgres.NewAdminRepo(pool), backups.Dir{Path: a.BackupDir}, time.Now)
		servers = append(servers, &http.Server{
			Addr:              a.Addr,
			Handler:           httpapi.NewAdminServer(admin, a.KeyHash, api.Metrics(), version, log).Handler(),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
		})
	}

	go deleteLeftGuilds(ctx, svc, log)

	errc := make(chan error, len(servers))
	for _, s := range servers {
		go func() {
			log.Info("listening", "addr", s.Addr)
			errc <- s.ListenAndServe()
		}()
	}
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, s := range servers {
			if err := s.Shutdown(shutdownCtx); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteLeftGuilds deletes, every hour, the servers the bot left more than
// usecase.LeftGuildRetention ago.
func deleteLeftGuilds(ctx context.Context, svc *usecase.Service, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		ids, err := svc.DeleteLeftGuilds(ctx)
		if err != nil {
			log.Error("delete left guilds", "err", err)
		} else if len(ids) > 0 {
			log.Info("deleted guilds the bot left", "guilds", ids)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
