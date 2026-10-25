# Docker Compose

1. Create a folder for Afterfeed and go into it:

   ```sh
   mkdir -p ~/afterfeed && cd ~/afterfeed
   ```

2. Create a file named `compose.yaml`:

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

   Change `PUID`, `PGID` and `TZ` to match your server (see [User and group](./#user-and-group)). To keep your memories on another disk, change `./data` to a full path, for example `/mnt/storage/afterfeed`.

3. Start Afterfeed:

   ```sh
   docker compose up -d
   ```

4. Open `http://<your-server>:8080` and follow the [Take-in](/guide/take-in).

## Thumbnails on a fast disk

If your data lives on a large, slow disk, add a cache volume on an SSD. The app feels faster and the bulk of your files stays on cheap storage:

```yaml
    volumes:
      - ./config:/config
      - /mnt/storage/afterfeed:/data
      - /mnt/ssd/afterfeed-cache:/cache
```

## Document previews

To see previews of Word, Excel and PowerPoint files in your messages, add the optional document service. See [Document previews](/configuration/document-previews).

## Updating

```sh
docker compose pull && docker compose up -d
```

See [Updating](/maintenance/updating).
