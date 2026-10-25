# Updating

Afterfeed updates its database automatically when a new version starts. Make a [backup](./backups) of the `config` folder before updating to a new major version.

## Docker Compose

```sh
docker compose pull
docker compose up -d
```

## Other setups

See the update steps in the guide for your setup: [Docker CLI](/install/docker-cli#updating), [Portainer](/install/portainer#updating), [Synology](/install/synology#updating), [QNAP](/install/qnap#updating), [TrueNAS](/install/truenas#updating) or [Unraid](/install/unraid#updating).

## Versions

Releases follow [semantic versioning](https://semver.org). Before version 1.0, a new minor version (for example 0.2 to 0.3) can contain breaking changes; they are listed in the release notes on GitHub.

To stay on one version, use a version tag such as `afterfeed/afterfeed:0.1` instead of `latest`.

## Importing again after an update

When a release adds support for more data, the release notes say so. Import your original export files again to get it. Afterfeed adds only what is new and keeps your edits.
