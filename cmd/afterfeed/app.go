package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/rudyh0/afterfeed/internal/auth"
	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/server"
	"github.com/rudyh0/afterfeed/internal/store"
)

// app wires the services together.
type app struct {
	cfg   *config.Config
	store *store.Store
	auth  *auth.Service
}

func newApp(ctx context.Context, cfg *config.Config, st *store.Store) (*app, error) {
	a := &app{cfg: cfg, store: st, auth: auth.New(st)}
	go a.housekeeping(ctx)
	return a, nil
}

func (a *app) deps() server.Deps {
	return server.Deps{Auth: a.auth}
}

func (a *app) Close() {}

// housekeeping removes expired sessions once an hour.
func (a *app) housekeeping(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := a.auth.CleanupSessions(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("cleanup sessions", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
