# Security model

Afterfeed holds very personal data. This page explains what it protects and what it does not.

## Where your data is

Everything stays on your server: the database, your photos and videos, and your messages. Afterfeed does not call external services, with two optional exceptions you choose yourself: the [map server](/configuration/map) and a [document preview service](/configuration/document-previews).

Afterfeed loads no fonts, scripts or images from other websites.

## Signing in

- Afterfeed has one account. Passwords are stored as salted PBKDF2-SHA256 hashes.
- After repeated failed attempts, sign-in is blocked for an increasing amount of time.
- Sessions are stored in the database and end after 30 days without use. Changing your password signs out every other session.

## Two-factor authentication

Optional. Turn it on under **Settings › Security** with any authenticator app that supports time-based codes (TOTP). When you turn it on, Afterfeed shows ten recovery codes. Each works once, instead of a code from the app.

If you lose both your device and your recovery codes, turn two-factor authentication off from the server:

```sh
docker exec -it afterfeed afterfeed auth reset-2fa
```

Anyone who can run commands in the container already controls the server and the data, so this does not weaken the protection.

## The vault

The vault PIN hides memories and conversations from the interface. It does **not** encrypt the files on disk. Anyone with access to your server or your data volume can open them. The vault is meant for the situation where you show your feed to someone, not for protection against someone with access to your server.

## Network

Afterfeed is meant for your own network. For access from elsewhere, use a VPN or a [reverse proxy with HTTPS](/install/reverse-proxy). Do not expose it to the internet over plain HTTP.

## Reporting a vulnerability

See [SECURITY.md](https://github.com/Rudyh0/Afterfeed/blob/main/SECURITY.md).
