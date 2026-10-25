# Backups

Back up two folders:

| Folder | Why |
| --- | --- |
| `config` | Your account, settings and the database |
| `data` | Your memories, messages and your edits |

The `cache` folder does not need a backup.

## Consistent database backups

The database is in WAL mode, so copying the files while Afterfeed runs can produce an inconsistent copy. Either stop Afterfeed during the backup:

```sh
docker compose stop afterfeed
# back up config and data
docker compose start afterfeed
```

or create a consistent copy of the database first and back that up:

```sh
docker exec afterfeed afterfeed backup /config/backup.db
```

## Your original exports

Your original export files are a backup too. Keep them separately; see [Keep your exports](/guide/keep-your-exports).

## Restoring

Stop Afterfeed, put the `config` and `data` folders back, and start it again. If only the `data` folder survived, start Afterfeed with an empty `config` folder, create your account, and run [`afterfeed index rebuild`](./commands#index-rebuild).
