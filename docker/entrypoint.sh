#!/bin/sh
# Prepares the volumes and starts Afterfeed as the user given by PUID and PGID,
# so files on the volumes belong to that user on the host.
set -eu

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"
CONFIG_DIR="${AFTERFEED_CONFIG_DIR:-/config}"
DATA_DIR="${AFTERFEED_DATA_DIR:-/data}"
CACHE_DIR="${AFTERFEED_CACHE_DIR:-}"

if [ "$(id -u)" = "0" ]; then
  mkdir -p "$CONFIG_DIR" "$DATA_DIR"

  # The config volume holds only Afterfeed's own files, so it can safely be
  # given to the right user. Only walk it when the owner is wrong.
  if [ "$(stat -c %u:%g "$CONFIG_DIR")" != "$PUID:$PGID" ]; then
    chown -R "$PUID:$PGID" "$CONFIG_DIR"
  fi
  if [ -z "$CACHE_DIR" ] && [ -d /cache ]; then
    CACHE_DIR=/cache
  fi
  if [ -n "$CACHE_DIR" ]; then
    mkdir -p "$CACHE_DIR"
    if [ "$(stat -c %u:%g "$CACHE_DIR")" != "$PUID:$PGID" ]; then
      chown -R "$PUID:$PGID" "$CACHE_DIR"
    fi
  fi

  # The data volume holds the user's files. Never change ownership of what is
  # already there; only make sure the top level is writable.
  if ! su-exec "$PUID:$PGID" test -w "$DATA_DIR"; then
    if [ -z "$(ls -A "$DATA_DIR" 2>/dev/null)" ]; then
      chown "$PUID:$PGID" "$DATA_DIR"
    else
      echo "afterfeed: $DATA_DIR is not writable for user $PUID:$PGID." >&2
      echo "afterfeed: set PUID and PGID to the owner of the folder on your host, or change its owner." >&2
      exit 1
    fi
  fi

  exec su-exec "$PUID:$PGID" /usr/local/bin/afterfeed-server "$@"
fi

exec /usr/local/bin/afterfeed-server "$@"
