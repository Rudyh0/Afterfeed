# Volumes and folders

## `/data` – your memories

The data volume is the source of truth. Everything in it is in open formats, so it stays usable without Afterfeed.

```txt
data/
├── import/                  Drop export files here for the Take-in
├── memories/
│   └── 2019/
│       └── 20191025-194700-facebook-3f9a2c1d/
│           ├── memory.json  Date, location, text, source and your edits
│           ├── 01.jpg
│           └── 02.jpg
└── messages/
    └── facebook/
        └── sanne-4e1b7a90/
            ├── conversation.json
            └── files/
```

- Every memory has its own folder named after its original date, with the media files and a `memory.json` file. The folder name does not change when you edit the date.
- `memory.json` holds the current values and, after an edit, the original values.
- Every conversation has a folder with a `conversation.json` file and its attachments.
- A hidden `.afterfeed` folder holds uploads in progress.

## `/config` – the database

```txt
config/
├── afterfeed.db       The index and your settings
├── afterfeed.db-wal
├── afterfeed.db-shm
└── cache/             Thumbnails and previews, when no /cache volume is mounted
```

The database is an index of the data volume plus your account and settings. If it is lost, rebuild the index with [`afterfeed index rebuild`](/maintenance/commands#index-rebuild); your account and settings then need to be set up again.

## `/cache` – previews

Thumbnails, previews and converted videos. Mount it on a fast disk if your data lives on a slow one. Everything here can be deleted; Afterfeed recreates it when needed.
