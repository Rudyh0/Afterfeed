package main

import (
	"context"

	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/server"
	"github.com/rudyh0/afterfeed/internal/store"
)

// app wires the services together.
type app struct {
	cfg   *config.Config
	store *store.Store
}

func newApp(ctx context.Context, cfg *config.Config, st *store.Store) (*app, error) {
	return &app{cfg: cfg, store: st}, nil
}

func (a *app) deps() server.Deps { return server.Deps{} }

func (a *app) Close() {}
