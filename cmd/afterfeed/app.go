package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/rudyh0/afterfeed/internal/auth"
	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/geo"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/importer/facebook"
	"github.com/rudyh0/afterfeed/internal/importer/instagram"
	"github.com/rudyh0/afterfeed/internal/importer/snapchat"
	"github.com/rudyh0/afterfeed/internal/importer/x"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
	"github.com/rudyh0/afterfeed/internal/server"
	"github.com/rudyh0/afterfeed/internal/store"
)

// app wires the services together.
type app struct {
	cfg     *config.Config
	store   *store.Store
	auth    *auth.Service
	library *library.Library
	media   *media.Service
	imports *importer.Manager
}

// importers lists the supported platforms, in the order they are shown.
func importers() []importer.Importer {
	return []importer.Importer{facebook.New(), instagram.New(), x.New(), snapchat.New()}
}

func newApp(ctx context.Context, cfg *config.Config, st *store.Store) (*app, error) {
	lib := library.New(cfg.MemoriesDir(), cfg.MessagesDir(), st)
	lib.SetGeocoder(geocoder)
	ms := media.New(cfg.CacheDir)
	a := &app{
		cfg:     cfg,
		store:   st,
		auth:    auth.New(st),
		library: lib,
		media:   ms,
		imports: importer.NewManager(cfg, st, lib, ms, importers()),
	}
	tools := ms.Tools()
	if tools.FFmpeg == "" || tools.VipsThumb == "" {
		slog.Warn("image or video tools not found; previews are limited", "ffmpeg", tools.FFmpeg != "", "vips", tools.VipsThumb != "")
	}
	a.imports.Start(ctx)
	go a.housekeeping(ctx)
	go func() {
		if n, err := lib.GeocodeMissing(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("name places", "err", err)
		} else if n > 0 {
			slog.Info("named places", "memories", n)
		}
	}()
	return a, nil
}

// geocoder names places with the embedded list of places, within 30 km.
func geocoder(lat, lon float64) (string, string, bool) {
	p, ok := geo.Default().Nearest(lat, lon, 30)
	return p.Name, p.Country, ok
}

func (a *app) deps() server.Deps {
	return server.Deps{Auth: a.auth, Imports: a.imports, Library: a.library, Media: a.media}
}

// Close waits for background work to stop.
func (a *app) Close() { a.imports.Wait() }

// housekeeping removes expired sessions and abandoned uploads once an hour.
func (a *app) housekeeping(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := a.auth.CleanupSessions(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("cleanup sessions", "err", err)
		}
		if err := a.imports.CleanupUploads(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("cleanup uploads", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
