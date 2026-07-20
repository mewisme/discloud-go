// Command discloud runs the DisCloud API server: unlimited cloud storage
// backed by Discord attachments, with PostgreSQL metadata and Valkey caching.
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

	"github.com/mewisme/discloud-go/internal/cache"
	"github.com/mewisme/discloud-go/internal/config"
	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/server"
	"github.com/mewisme/discloud-go/internal/store"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
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

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	ca, err := cache.New(cfg.ValkeyURL)
	if err != nil {
		return err
	}
	defer ca.Close()

	dc := discord.New(cfg.DiscordBotToken, cfg.DiscordChannelID)
	if err := st.EnsureBots(ctx, dc.TokenCount()); err != nil {
		return err
	}
	srv := server.New(log, st, ca, dc, cfg.PublicBaseURL)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "port", cfg.Port, "version", version, "discord_bots", dc.TokenCount())
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
