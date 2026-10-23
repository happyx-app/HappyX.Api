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

	"github.com/moixblau/HappyX.Api/internal/config"
	"github.com/moixblau/HappyX.Api/internal/discord"
	"github.com/moixblau/HappyX.Api/internal/repository/postgres"
	httpapi "github.com/moixblau/HappyX.Api/internal/transport/http"
	"github.com/moixblau/HappyX.Api/internal/usecase"
)

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
		postgres.NewMoodRepo(pool), postgres.NewDispatchRepo(pool),
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
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}
