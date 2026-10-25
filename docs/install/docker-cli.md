# Docker CLI

```sh
docker run -d \
  --name afterfeed \
  -e PUID=1000 \
  -e PGID=1000 \
  -e TZ=Europe/Amsterdam \
  -v /path/to/afterfeed/config:/config \
  -v /path/to/afterfeed/data:/data \
  -p 8080:8080 \
  --restart unless-stopped \
  afterfeed/afterfeed:latest
```

Replace `/path/to/afterfeed` with a folder on your server and set `PUID`, `PGID` and `TZ` (see [User and group](./#user-and-group)).

Open `http://<your-server>:8080` and follow the [Take-in](/guide/take-in).

## Updating

```sh
docker pull afterfeed/afterfeed:latest
docker stop afterfeed && docker rm afterfeed
```

Then run the `docker run` command above again. Your data stays in the folders you mounted.
