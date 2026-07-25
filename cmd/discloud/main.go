// Command discloud runs the DisCloud API server: unlimited cloud storage
// backed by Discord attachments, with PostgreSQL or SQLite metadata and
// Valkey or in-memory caching.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mewisme/discloud-go/internal/appconfig"
	"github.com/mewisme/discloud-go/internal/auth"
	"github.com/mewisme/discloud-go/internal/cache"
	"github.com/mewisme/discloud-go/internal/config"
	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/server"
	"github.com/mewisme/discloud-go/internal/serverconfig"
	pgxstore "github.com/mewisme/discloud-go/internal/store/pgx"
	sqlitestore "github.com/mewisme/discloud-go/internal/store/sqlite"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			os.Exit(1)
		}
		return
	}

	cfg, err := loadServerConfig()
	logLevel := slog.LevelInfo
	if err == nil {
		logLevel = cfg.SlogLevel()
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	if err != nil {
		log.Error("fatal", "error", err)
		if errors.Is(err, config.ErrInvalid) {
			os.Exit(0)
		}
		os.Exit(1)
	}
	if err := run(log, cfg); err != nil {
		log.Error("fatal", "error", err)
		if errors.Is(err, config.ErrInvalid) {
			os.Exit(0)
		}
		os.Exit(1)
	}
}

func loadServerConfig() (config.Config, error) {
	sources := config.Sources{
		Defaults: config.DefaultsMap(),
		Process:  config.ProcessEnvMap(),
	}
	configDir := strings.TrimSpace(os.Getenv("DISCLOUD_CONFIG_DIR"))
	if configDir == "" {
		if d, err := config.ResolveConfigDir(); err == nil {
			if _, err := os.Stat(filepath.Join(d, serverconfig.YAMLName)); err == nil {
				configDir = d
			}
		}
	}
	if configDir != "" {
		if data, err := os.ReadFile(filepath.Join(configDir, serverconfig.YAMLName)); err == nil {
			var f serverconfig.File
			if err := serverconfig.UnmarshalYAML(data, &f); err != nil {
				return config.Config{}, fmt.Errorf("%w: %w", config.ErrInvalid, err)
			}
			sources.YAML = serverconfig.EnvMap(f)
		} else if !os.IsNotExist(err) {
			return config.Config{}, err
		}
		if m, err := serverconfig.ParseDotEnvFile(filepath.Join(configDir, serverconfig.EnvName)); err != nil {
			return config.Config{}, err
		} else {
			sources.DotEnv = m
		}
	}
	return config.LoadFromSources(sources)
}

func run(log *slog.Logger, cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, closer, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer closer()

	ca, cacheCloser, err := openCache(cfg)
	if err != nil {
		return err
	}
	defer cacheCloser()

	dc := discord.New(cfg.DiscordBotToken, cfg.DiscordChannelID)
	if err := st.EnsureBots(ctx, dc.TokenCount()); err != nil {
		return err
	}
	salt, err := config.LoadOrCreateVisitorHashSalt(cfg.VisitorSecretPath())
	if err != nil {
		return err
	}

	chunkSize, tier, err := appconfig.ResolveEffectiveChunk(ctx, dc, cfg.DiscordGuildID, cfg.PreferredChunkSize)
	if err != nil {
		return fmt.Errorf("resolve chunk size: %w", err)
	}
	if cfg.DiscordGuildID != "" {
		log.Info("premium tier resolved",
			"guild_id", cfg.DiscordGuildID,
			"premium_tier", tier,
			"chunk_size", chunkSize,
		)
		persistLocalTier(cfg, tier, cfg.PreferredChunkSize)
	} else {
		log.Info("using legacy chunk size (no guild_id)", "chunk_size", chunkSize)
	}

	useDB := cfg.Backend == config.BackendPostgres
	srv := server.New(log, st, ca, dc, server.Options{
		PublicBaseURL:           cfg.APIURL,
		VisitorSalt:             salt,
		WebOrigin:               cfg.WebOrigin,
		CookieSecure:            cfg.CookieSecure,
		TrustProxy:              cfg.TrustProxy,
		Keys:                    auth.DeriveKeys(cfg.AppSecret),
		ChunkSize:               chunkSize,
		GuildID:                 cfg.DiscordGuildID,
		PreferredChunkSize:      cfg.PreferredChunkSize,
		UseDBConfig:             useDB,
		EnvPublic:               envPublicFromConfig(cfg),
		RateLimitUploadPerMin:   cfg.RateLimitUploadPerMin,
		RateLimitDownloadPerMin: cfg.RateLimitDownloadPerMin,
		MaxUserBytes:            cfg.MaxUserBytes,
		MaxAnonUploadsPerDay:    cfg.MaxAnonUploadsPerDay,
		MaxAnonBytesPerDay:      cfg.MaxAnonBytesPerDay,
		MaxRawUploadBytes:       cfg.MaxRawUploadBytes,
		CaptchaSecret:           cfg.CaptchaSecret,
	})
	if useDB {
		if err := srv.RefreshPublicConfig(ctx); err != nil {
			return fmt.Errorf("load public config from database: %w", err)
		}
	}
	go srv.RunCleanup(ctx)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening",
			"addr", cfg.ListenAddr(),
			"version", version,
			"discord_bots", dc.TokenCount(),
			"store", cfg.StoreBackendName(),
			"cache", cfg.CacheBackendName(),
		)
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

func envPublicFromConfig(cfg config.Config) map[string]string {
	return map[string]string{
		appconfig.KeyGuildID:        cfg.DiscordGuildID,
		appconfig.KeyChunkSize:      strconv.FormatInt(cfg.PreferredChunkSize, 10),
		appconfig.KeyMaxRawUpload:   strconv.FormatInt(cfg.MaxRawUploadBytes, 10),
		appconfig.KeyRLUpload:       strconv.Itoa(cfg.RateLimitUploadPerMin),
		appconfig.KeyRLDownload:     strconv.Itoa(cfg.RateLimitDownloadPerMin),
		appconfig.KeyMaxUserBytes:   strconv.FormatInt(cfg.MaxUserBytes, 10),
		appconfig.KeyMaxAnonUploads: strconv.Itoa(cfg.MaxAnonUploadsPerDay),
		appconfig.KeyMaxAnonBytes:   strconv.FormatInt(cfg.MaxAnonBytesPerDay, 10),
	}
}

// persistLocalTier writes premiumTier (and preferred chunkSize if unset) back to server.yaml.
func persistLocalTier(cfg config.Config, tier int, preferred int64) {
	if cfg.Backend != config.BackendSQLite {
		return
	}
	configDir := strings.TrimSpace(os.Getenv("DISCLOUD_CONFIG_DIR"))
	if configDir == "" {
		return
	}
	installRoot := filepath.Dir(configDir)
	f, err := serverconfig.Load(installRoot)
	if err != nil {
		return
	}
	f.PremiumTier = tier
	if f.GuildID == "" {
		f.GuildID = cfg.DiscordGuildID
	}
	if f.ChunkSize <= 0 && preferred > 0 {
		f.ChunkSize = preferred
	}
	_ = serverconfig.Save(installRoot, f)
}

func openStore(ctx context.Context, cfg config.Config) (server.Store, func(), error) {
	info, err := config.ParseDatabaseURL(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", config.ErrInvalid, err)
	}
	switch info.Backend {
	case config.BackendPostgres:
		st, err := pgxstore.New(ctx, info.DriverDSN)
		if err != nil {
			return nil, nil, err
		}
		if err := st.Migrate(ctx); err != nil {
			st.Close()
			return nil, nil, err
		}
		return st, st.Close, nil
	case config.BackendSQLite:
		st, err := sqlitestore.New(ctx, info.DriverDSN)
		if err != nil {
			return nil, nil, err
		}
		if err := st.Migrate(ctx); err != nil {
			_ = st.Close()
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("%w: unsupported database backend", config.ErrInvalid)
	}
}

func openCache(cfg config.Config) (server.Cache, func(), error) {
	if cfg.ValkeyURL == "" {
		if cfg.Backend != config.BackendSQLite {
			return nil, nil, fmt.Errorf("%w: DISCLOUD_VALKEY_URL is required for PostgreSQL", config.ErrInvalid)
		}
		m := cache.NewMemory()
		return m, m.Close, nil
	}
	ca, err := cache.New(cfg.ValkeyURL)
	if err != nil {
		return nil, nil, err
	}
	return ca, ca.Close, nil
}
