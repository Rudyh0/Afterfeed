# Commands

Afterfeed has a few maintenance commands. Run them inside the container:

```sh
docker exec -it afterfeed afterfeed <command>
```

Replace the first `afterfeed` with your container name if it is different. The commands run as the same user as the server.

## `auth reset-2fa`

Turns off two-factor authentication, for when you lost your phone and your recovery codes.

```sh
docker exec -it afterfeed afterfeed auth reset-2fa
```

Sign in with your password and turn two-factor authentication on again under **Settings › Security**.

## `auth reset-password`

Sets a new temporary password and prints it. Afterfeed asks you to choose a new password after signing in with it. All sessions are signed out.

```sh
docker exec -it afterfeed afterfeed auth reset-password
```

## `auth reset-vault-pin`

Removes the vault PIN. The vault keeps its contents; you set a new PIN the next time you open it.

```sh
docker exec -it afterfeed afterfeed auth reset-vault-pin
```

## `index rebuild`

Rebuilds the database index from the files on the data volume. Use it after restoring the data folder without the database, or when the index is out of sync. Your account and settings are kept.

```sh
docker exec -it afterfeed afterfeed index rebuild
```

## `backup`

Writes a consistent copy of the database to a file, while Afterfeed keeps running.

```sh
docker exec afterfeed afterfeed backup /config/backup.db
```

## `version`

```sh
docker exec afterfeed afterfeed version
```

Every reset is written to the log.
