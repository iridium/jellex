// Command jellex emulates the Plex Media Server API on top of Jellyfin.
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/iridium/jellex/internal/config"
	"github.com/iridium/jellex/internal/myplex"
	"github.com/iridium/jellex/internal/plex"
	jellyfin "github.com/sj14/jellyfin-go/api"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// waitForJellyfin blocks until Jellyfin answers, since jellex's identity is
// derived from it and nothing works without it.
func waitForJellyfin(ctx context.Context, jf *jellyfin.APIClient, url string) (*jellyfin.PublicSystemInfo, error) {
	for {
		info, _, err := jf.SystemAPI.GetPublicSystemInfo(ctx).Execute()
		if err == nil {
			return info, nil
		}
		slog.Warn("waiting for jellyfin", "url", url, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	jf := jellyfin.NewAPIClient(&jellyfin.Configuration{
		Servers:       jellyfin.ServerConfigurations{{URL: strings.TrimRight(cfg.JellyfinURL, "/")}},
		DefaultHeader: map[string]string{"Authorization": fmt.Sprintf(`MediaBrowser Token="%s"`, cfg.JellyfinAPIKey)},
		UserAgent:     "jellex",
		HTTPClient:    &http.Client{Timeout: 15 * time.Second},
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info, err := waitForJellyfin(ctx, jf, cfg.JellyfinURL)
	if err != nil {
		return err
	}
	slog.Info("connected to jellyfin", "name", info.GetServerName(), "version", info.GetVersion())
	if cfg.MachineID == "" {
		// Stable per Jellyfin server, in the 40-hex form PMS uses.
		sum := sha1.Sum([]byte("jellex:" + info.GetId()))
		cfg.MachineID = hex.EncodeToString(sum[:])
	}
	slog.Info("server identity", "name", cfg.ServerName, "machineIdentifier", cfg.MachineID)

	mp, err := myplex.Open(cfg.DataDir, myplex.Identity{MachineID: cfg.MachineID, Name: cfg.ServerName, Version: plex.Version})
	if err != nil {
		return err
	}
	if _, claimed := mp.Claimed(); !claimed && cfg.PlexClaim != "" {
		if err := mp.Claim(ctx, cfg.PlexClaim); err != nil {
			slog.Error("claim with PLEX_CLAIM failed", "err", err)
		}
	}
	go mp.PublishLoop(ctx, cfg.PublishURLs, 15*time.Minute)

	handler, err := plex.NewServer(ctx, cfg, jf, mp)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("jellex listening", "addr", cfg.ListenAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
