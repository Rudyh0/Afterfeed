# Environment variables

Afterfeed needs no configuration file. Everything has a default that works in the Docker image. Settings you change day to day, such as the language, the theme and the map, are in the app under **Settings**.

| Variable | Default | Description |
| --- | --- | --- |
| `PUID` | `1000` | User ID that owns the files on the volumes |
| `PGID` | `1000` | Group ID that owns the files on the volumes |
| `TZ` | `UTC` | Time zone of the server, used for dates without a time zone |
| `AFTERFEED_PORT` | `8080` | Port inside the container |
| `AFTERFEED_CONFIG_DIR` | `/config` | Database and settings |
| `AFTERFEED_DATA_DIR` | `/data` | Memories, messages and the import folder |
| `AFTERFEED_CACHE_DIR` | `/cache` if mounted, otherwise `/config/cache` | Thumbnails and previews |
| `AFTERFEED_IMPORT_DIR` | `/data/import` | Folder that the Take-in reads export files from |
| `AFTERFEED_PREVIEW_URL` | | URL of a Gotenberg service for [document previews](./document-previews) |
| `AFTERFEED_SECURE_COOKIES` | `auto` | `auto` marks cookies secure when the request came over HTTPS. Set `true` to always require HTTPS, `false` to never |
| `AFTERFEED_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
