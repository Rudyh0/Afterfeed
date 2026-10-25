# Afterfeed

Your memories, without the platform.

Afterfeed is a self-hosted archive for the photos, videos, posts and messages you exported from Facebook, Instagram, X and Snapchat. It runs in a single container on your own hardware.

- **Documentation:** https://rudyh0.github.io/Afterfeed/
- **Source code:** https://github.com/Rudyh0/Afterfeed

## Quick start

```yaml
services:
  afterfeed:
    image: afterfeed/afterfeed:latest
    container_name: afterfeed
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Europe/Amsterdam
    volumes:
      - ./config:/config
      - ./data:/data
    ports:
      - 8080:8080
    restart: unless-stopped
```

Open `http://<your-server>:8080` and follow the Take-in.

## Volumes

| Path | Contents |
| --- | --- |
| `/config` | Database and settings |
| `/data` | Your memories, messages and the import folder |
| `/cache` | Optional. Thumbnails and previews, safe to delete |

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `PUID`, `PGID` | `1000` | Owner of the files on the volumes |
| `TZ` | `UTC` | Time zone |
| `AFTERFEED_PORT` | `8080` | Port inside the container |
| `AFTERFEED_PREVIEW_URL` | | Optional Gotenberg URL for document previews |

Supported architectures: `amd64` and `arm64`.

License: AGPL-3.0.
