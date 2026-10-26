<p align="center">
  <img src="docs/public/logo-light.svg#gh-light-mode-only" alt="Afterfeed" width="200">
  <img src="docs/public/logo-dark.svg#gh-dark-mode-only" alt="Afterfeed" width="200">
</p>

<h1 align="center">Afterfeed</h1>

<p align="center">Your memories, without the platform.</p>

<p align="center">
  <a href="https://rudyh0.github.io/Afterfeed/">Documentation</a> ·
  <a href="https://rudyh0.github.io/Afterfeed/install/">Install</a> ·
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

---

Afterfeed is a self-hosted archive for the photos, videos and posts you exported from social media. Import your data exports, close your accounts, and keep looking back at your own life on your own server.

It is not a social network. There are no followers, likes, comments or sharing, and no algorithm deciding what you see. Everything runs in a single Docker container on your own hardware, and your archive never leaves it.

## Features

- **On this day** – everything from today's date in previous years, with arrows to move between days
- **Years** – every year at a glance, month by month
- **Search** – across text, places, dates and sources, in five languages
- **Map** – browse your memories by where they happened
- **Messages** – a read-only archive of your conversations per platform, including voice messages and documents
- **Vault** – keep memories and conversations out of sight behind a PIN
- **Metadata editor** – fix dates, times and locations for one memory or a whole selection, with the original values kept
- **Downloads** – save any photo or video to your device, with the correct date and location written into the file
- **New memories** – add your own, privately
- **Two-factor authentication** – optional, with recovery codes
- **Languages** – English, Dutch, French, German and Spanish
- **Light and dark mode**

## Supported platforms

| Platform | Posts | Photos and videos | Messages |
| --- | --- | --- | --- |
| Facebook | ✓ | ✓ | ✓ (including end-to-end encrypted chats) |
| Instagram | ✓ | ✓ | ✓ |
| X | ✓ | ✓ | ✓ |
| Snapchat | ✓ (Memories) | ✓ | ✓ (saved chats) |

Afterfeed guides you through requesting each export, including which options to select.

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

```sh
docker compose up -d
```

Then open `http://<your-server>:8080` and follow the Take-in to create your account and import your first export.

The image is also available as `ghcr.io/rudyh0/afterfeed`. See the [installation guide](https://rudyh0.github.io/Afterfeed/install/) for Portainer, Synology, QNAP, TrueNAS and Unraid.

## Keep your original exports

Always keep a copy of the export files you downloaded somewhere other than Afterfeed. Once you close an account, you cannot request the data again. With the original files you can re-import into a newer version of Afterfeed, or move to another tool entirely.

## Status

Afterfeed is in early development. Expect breaking changes before version 1.0.

## Contributing

Bug fixes, translations and documentation improvements are welcome. For new features, please open an issue first. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Please report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

## License

Afterfeed is licensed under the [GNU Affero General Public License v3.0](LICENSE).

Place names come from [GeoNames](https://www.geonames.org), licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). The default map is [OpenFreeMap](https://openfreemap.org) with data from [OpenStreetMap](https://www.openstreetmap.org/copyright) contributors. Text in photos is found with the PP-OCRv3 detection model from [PaddleOCR](https://github.com/PaddlePaddle/PaddleOCR), as converted by [RapidOCR](https://github.com/RapidAI/RapidOCR), and read with [Tesseract](https://github.com/tesseract-ocr/tesseract), all licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
