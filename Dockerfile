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
      exiftool \
      tesseract-ocr

# Language models for reading text in photos (tessdata_fast 4.1.0): small
# and quick, which suits captions and signs. AFTERFEED_OCR_LANGUAGES picks
# which of them are used.
ARG TESSDATA=https://raw.githubusercontent.com/tesseract-ocr/tessdata_fast/4.1.0
ADD --chmod=0644 --checksum=sha256:7d4322bd2a7749724879683fc3912cb542f19906c83bcc1a52132556427170b2 ${TESSDATA}/eng.traineddata /usr/share/tessdata/eng.traineddata
ADD --chmod=0644 --checksum=sha256:ced0e5e046a84c908a6aa7accbef9a232c4a5d9a8276691b81c6ee64d02963f6 ${TESSDATA}/nld.traineddata /usr/share/tessdata/nld.traineddata
ADD --chmod=0644 --checksum=sha256:19d219bbb6672c869d20a9636c6816a81eb9a71796cb93ebe0cb1530e2cdb22d ${TESSDATA}/deu.traineddata /usr/share/tessdata/deu.traineddata
ADD --chmod=0644 --checksum=sha256:ced037562e8c80c13122dece28dd477d399af80911a28791a66a63ac1e3445ca ${TESSDATA}/fra.traineddata /usr/share/tessdata/fra.traineddata
ADD --chmod=0644 --checksum=sha256:6f2e04d02774a18f01bed44b1111f2cd7f3ba7ac9dc4373cd3f898a40ea6b464 ${TESSDATA}/spa.traineddata /usr/share/tessdata/spa.traineddata
COPY --from=server /out/afterfeed-server /usr/local/bin/afterfeed-server
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY docker/afterfeed.sh /usr/local/bin/afterfeed
RUN chmod 0755 /usr/local/bin/entrypoint.sh /usr/local/bin/afterfeed /usr/local/bin/afterfeed-server

ENV TESSDATA_PREFIX=/usr/share/tessdata \
    OMP_THREAD_LIMIT=1 \
    AFTERFEED_CONFIG_DIR=/config \
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
