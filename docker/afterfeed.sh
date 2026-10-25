#!/bin/sh
# Wrapper for maintenance commands run with `docker exec`. It runs them as the
# same user as the server, so they never create files the server cannot open.
set -eu

if [ "$(id -u)" = "0" ]; then
  exec su-exec "${PUID:-1000}:${PGID:-1000}" /usr/local/bin/afterfeed-server "$@"
fi
exec /usr/local/bin/afterfeed-server "$@"
