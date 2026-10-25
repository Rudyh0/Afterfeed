# Security policy

## Reporting a vulnerability

Please do not report security issues in public issues or discussions.

Use GitHub's private reporting instead: open the **Security** tab of this repository and choose **Report a vulnerability**. You will get a response as soon as possible, usually within a week.

Please include:

- The Afterfeed version
- How you installed it (Docker CLI, Portainer, a NAS, and so on)
- Steps to reproduce the issue
- What an attacker could do with it

## Supported versions

Only the latest release receives security fixes.

## Scope notes

- **The vault PIN protects the interface, not the files.** Anyone with access to the server or the `/data` volume can read the files in it. This is by design and not considered a vulnerability.
- **Afterfeed is meant to run on a trusted network** or behind a reverse proxy with HTTPS. Exposing it directly to the internet over plain HTTP is not supported.
