# Troubleshooting

## Logs

```sh
docker logs afterfeed
```

For more detail, set `AFTERFEED_LOG_LEVEL=debug` and restart the container.

## "is not writable for user"

Afterfeed runs as `PUID:PGID` and cannot write to the data folder. Set `PUID` and `PGID` to the owner of the folder on your server (see [User and group](/install/#user-and-group)), or change the owner of the folder:

```sh
sudo chown -R 1000:1000 /path/to/afterfeed/data
```

## The page does not load

- Check that the container runs: `docker ps`.
- Check the port mapping. Afterfeed listens on `8080` inside the container.
- If you use a reverse proxy, see [Reverse proxy and HTTPS](/install/reverse-proxy).

## An upload stops

Uploads continue where they stopped when you add the same file again. Behind a reverse proxy, make sure request bodies of at least 32 MB are allowed. For very large exports, use the [import folder](/guide/large-exports).

## An import seems to take forever

A large export can take a while, mostly for copying the photos and videos. Follow it in the log:

```sh
docker logs -f afterfeed
```

While an import runs, Afterfeed writes a line every 15 seconds with the phase, how many items are done and an estimate of the time left:

```
level=INFO msg="import progress" id=1 phase=copying percent=34 done=2140 total=6150 per_minute=1800 left=2m10s
```

When nothing has moved for two minutes, a warning names the file it is waiting for:

```
level=WARN msg="import has not moved" id=1 phase=copying for=2m0s file=memories/2019-06-01_….mp4 file_for=2m5s
```

If the import stays at the same file, [open an issue](https://github.com/Rudyh0/Afterfeed/issues) with these log lines.

When Afterfeed restarts during an import, the import starts again by itself. Everything that was already copied is recognised and skipped.

## An import reports unreadable files

The report lists files Afterfeed could not read, for example damaged photos or a format it does not know. The rest of the import continues. If many files are affected, or a whole year is missing, [open an issue](https://github.com/Rudyh0/Afterfeed/issues) and describe the structure of the export, without attaching personal data.

## Photos show the wrong date

Some platforms store the upload date instead of the date the photo was taken. Fix it with the [metadata editor](/guide/metadata), for one photo or a whole selection.

## Locked out

- Lost your two-factor device and recovery codes: [`auth reset-2fa`](./commands#auth-reset-2fa)
- Forgot your password: [`auth reset-password`](./commands#auth-reset-password)
- Forgot the vault PIN: [`auth reset-vault-pin`](./commands#auth-reset-vault-pin)
