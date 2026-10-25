# Install

Afterfeed runs as a single Docker container. Pick the guide for your setup:

- [Docker Compose](./docker-compose) – the recommended way on any Linux server
- [Docker CLI](./docker-cli) – a single `docker run` command
- [Portainer](./portainer)
- [Synology](./synology) – Container Manager
- [QNAP](./qnap) – Container Station
- [TrueNAS](./truenas) – Apps
- [Unraid](./unraid)

After installing, open `http://<your-server>:8080` in a browser and follow the [Take-in](/guide/take-in).

## Images

The same image is published to two registries:

| Registry | Image |
| --- | --- |
| Docker Hub | `afterfeed/afterfeed` |
| GitHub Container Registry | `ghcr.io/rudyh0/afterfeed` |

Both support `amd64` and `arm64`, so the image also runs on a Raspberry Pi 4 or 5 and on ARM-based NAS systems.

| Tag | Meaning |
| --- | --- |
| `latest` | The newest stable release |
| `0.1`, `0.1.0` | A specific version |

## What you need

- **Docker** with Compose, or a NAS app that runs Docker containers
- **Two folders** on your server: one for settings (`config`) and one for your memories (`data`)
- **Free disk space** of roughly the size of your exports, plus the export files themselves while they are being uploaded. Photos and videos are copied out of the archive once; the previews Afterfeed creates take a few percent extra.
- **Memory:** 1 GB is enough for normal use. Imports of very large exports are faster with more.

## Volumes

| Container path | Contents | Back up? |
| --- | --- | --- |
| `/config` | Database and settings | Yes |
| `/data` | Your memories, messages and the import folder | Yes |
| `/cache` | Optional. Thumbnails and previews | No, Afterfeed recreates them |

See [Volumes and folders](/configuration/volumes) for what is inside each folder.

## User and group

Set `PUID` and `PGID` to the user and group that own the folders on your server. Afterfeed runs as that user, so every file it creates belongs to you. On most Linux systems the first user is `1000:1000`. Run `id` on the server to check:

```sh
id
# uid=1000(you) gid=1000(you) ...
```

The NAS guides list the usual values for each system.

## Access from outside your network

Afterfeed is meant for your own network. To reach it from elsewhere, use a VPN such as WireGuard or Tailscale, or put it behind a [reverse proxy with HTTPS](./reverse-proxy). Do not expose port 8080 directly to the internet.
