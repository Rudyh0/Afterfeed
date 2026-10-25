GO_TAGS := sqlite_fts5
VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/rudyh0/afterfeed/internal/version.Version=$(VERSION) \
	-X github.com/rudyh0/afterfeed/internal/version.Commit=$(COMMIT) \
	-X github.com/rudyh0/afterfeed/internal/version.Date=$(DATE)

DEV_ENV := AFTERFEED_CONFIG_DIR=$(CURDIR)/dev-data/config \
	AFTERFEED_DATA_DIR=$(CURDIR)/dev-data/data \
	AFTERFEED_DEV=1 \
	AFTERFEED_LOG_LEVEL=debug

.PHONY: build web server dev dev-server dev-web test test-go test-web check docs clean

build: web server

web:
	cd web && npm ci && npm run build

server:
	CGO_ENABLED=1 go build -tags "$(GO_TAGS)" -ldflags "$(LDFLAGS)" -o bin/afterfeed ./cmd/afterfeed

# Runs the backend and the Vite dev server. Open http://localhost:5173.
dev:
	$(MAKE) -j2 dev-server dev-web

dev-server:
	$(DEV_ENV) go run -tags "$(GO_TAGS)" ./cmd/afterfeed serve

dev-web:
	cd web && npm run dev

test: test-go test-web

test-go:
	go test -tags "$(GO_TAGS)" ./...

test-web:
	cd web && npm test

check:
	gofmt -l . | (! grep .) || (echo "run gofmt" && exit 1)
	go vet -tags "$(GO_TAGS)" ./...
	cd web && npm run check

docs:
	cd docs && npm ci && npm run dev

clean:
	rm -rf bin web/dist internal/web/dist/assets internal/web/dist/index.html
