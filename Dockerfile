# syntax=docker/dockerfile:1

# --- Web app -----------------------------------------------------------------
# Built once on the build machine's own architecture; the output is plain files.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN mkdir -p /src/internal/web/dist && npm run build

# --- Server ------------------------------------------------------------------
# Built for each target architecture. The SQLite driver needs cgo.
FROM golang:1.24-alpine AS server
RUN apk add --no-cache build-base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/web/dist/ internal/web/dist/
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=1 go build -tags sqlite_fts5 -trimpath \
      -ldflags "-s -w \
        -X github.com/rudyh0/afterfeed/internal/version.Version=${VERSION} \
        -X github.com/rudyh0/afterfeed/internal/version.Commit=${COMMIT} \
        -X github.com/rudyh0/afterfeed/internal/version.Date=${DATE}" \
      -o /out/afterfeed-server ./cmd/afterfeed

# --- Runtime -----------------------------------------------------------------
FROM alpine:3.22
RUN apk add --no-cache \
      ca-certificates \
      tzdata \
      su-exec \
      ffmpeg \
      vips-tools \
      vips-heif \
      vips-poppler \
      exiftool
COPY --from=server /out/afterfeed-server /usr/local/bin/afterfeed-server
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY docker/afterfeed.sh /usr/local/bin/afterfeed
RUN chmod 0755 /usr/local/bin/entrypoint.sh /usr/local/bin/afterfeed /usr/local/bin/afterfeed-server

ENV AFTERFEED_CONFIG_DIR=/config \
    AFTERFEED_DATA_DIR=/data \
    AFTERFEED_PORT=8080 \
    PUID=1000 \
    PGID=1000

VOLUME ["/config", "/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD ["afterfeed", "healthcheck"]

ENTRYPOINT ["entrypoint.sh"]
CMD ["serve"]
