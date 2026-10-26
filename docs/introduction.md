# What is Afterfeed?

Afterfeed is a self-hosted archive for your social media memories. You request a data export from a platform, import it into Afterfeed, and keep looking back at your photos, videos, posts and messages after you close the account.

Afterfeed is not a social network. It has no followers, likes, comments, sharing or notifications, and no algorithm. It shows your own memories, in order, when you ask for them.

## What you get

- **Today** shows everything from today's date in previous years. Arrows move to the previous or next day.
- **Years** shows every year month by month.
- **Search** finds memories by words, places, dates and seasons, such as "summer 2019" or "Lisbon".
- **Map** shows where your memories happened.
- **Messages** is a read-only archive of your conversations per platform, including voice messages and documents. Messages never appear in Today or Years.
- **Vault** keeps memories and conversations out of every other view, behind a PIN.
- **Metadata editor** fixes dates, times and locations for one memory or a selection. The original values are kept.
- **Downloads** save a photo or video to your device with the date and location written into the file.
- **New memories** can be added by you, for yourself.

## Supported platforms

| Platform | What Afterfeed imports |
| --- | --- |
| Facebook | Posts, photos, videos, check-ins and messages, including end-to-end encrypted chats |
| Instagram | Posts, stories, reels and messages |
| X | Posts, photos, videos and direct messages |
| Snapchat | Memories and saved chats |

## How it runs

Afterfeed is a single Docker container with a built-in database. It needs no external services. Your archive stays on your server, in a folder structure you can read without Afterfeed. The only outside connection is the map, which loads map images from a map server; you can [run your own or turn the map off](/configuration/map).

Afterfeed is aimed at people who already run Docker, for example on a home server or a NAS. See [Install](/install/) to get started.

## Languages

The app is available in English, Dutch, French, German and Spanish. This documentation is in English.

## Credits

Place names come from [GeoNames](https://www.geonames.org), licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). The default map is [OpenFreeMap](https://openfreemap.org) with data from [OpenStreetMap](https://www.openstreetmap.org/copyright) contributors. Text in photos is found with the PP-OCRv3 detection model from [PaddleOCR](https://github.com/PaddlePaddle/PaddleOCR), as converted by [RapidOCR](https://github.com/RapidAI/RapidOCR), and read with [Tesseract](https://github.com/tesseract-ocr/tesseract), all licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
