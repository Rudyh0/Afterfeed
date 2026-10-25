package server

import (
	"testing"

	"github.com/rudyh0/afterfeed/internal/auth"
	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/store"
)

// testDeps builds the services for API tests.
func testDeps(t *testing.T, cfg *config.Config, st *store.Store) Deps {
	t.Helper()
	return Deps{Auth: auth.New(st)}
}
