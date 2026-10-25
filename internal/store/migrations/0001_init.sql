-- Settings edited in the web interface, stored as JSON values.
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Afterfeed has a single account. The table allows more rows so a reset or a
-- future household mode does not need a schema change.
CREATE TABLE users (
    id                   INTEGER PRIMARY KEY,
    username             TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    password_hash        TEXT    NOT NULL,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    totp_secret          TEXT,
    totp_enabled         INTEGER NOT NULL DEFAULT 0,
    totp_last_step       INTEGER NOT NULL DEFAULT 0,
    vault_pin_hash       TEXT,
    locale               TEXT    NOT NULL DEFAULT 'en',
    created_at           TEXT    NOT NULL,
    updated_at           TEXT    NOT NULL
);

CREATE TABLE recovery_codes (
    id        INTEGER PRIMARY KEY,
    user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT    NOT NULL,
    used_at   TEXT
);

CREATE TABLE sessions (
    token_hash      TEXT    PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mfa_pending     INTEGER NOT NULL DEFAULT 0,
    vault_unlocked  TEXT,
    created_at      TEXT    NOT NULL,
    last_seen_at    TEXT    NOT NULL,
    expires_at      TEXT    NOT NULL,
    user_agent      TEXT
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

-- Failed attempts per key (for example "login:<ip>" or "vault:<user>"), used
-- for increasing delays after repeated failures.
CREATE TABLE attempts (
    key             TEXT    PRIMARY KEY,
    failures        INTEGER NOT NULL DEFAULT 0,
    locked_until    TEXT,
    last_failure_at TEXT
);

-- One row per import run. An import can consist of several archive parts.
CREATE TABLE imports (
    id          INTEGER PRIMARY KEY,
    platform    TEXT    NOT NULL,
    status      TEXT    NOT NULL,
    origin      TEXT    NOT NULL,
    files_json  TEXT    NOT NULL DEFAULT '[]',
    progress    REAL    NOT NULL DEFAULT 0,
    phase       TEXT,
    report_json TEXT,
    error       TEXT,
    created_at  TEXT    NOT NULL,
    started_at  TEXT,
    finished_at TEXT
);

-- Uploads in progress. Files are written in chunks so an interrupted upload can
-- continue where it stopped.
CREATE TABLE uploads (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    received   INTEGER NOT NULL DEFAULT 0,
    platform   TEXT,
    status     TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

-- The index of memories. Every memory also has a memory.json file in its
-- folder under /data/memories, which is the source of truth.
CREATE TABLE memories (
    id           TEXT    PRIMARY KEY,
    source_key   TEXT    NOT NULL UNIQUE,
    platform     TEXT    NOT NULL,
    kind         TEXT    NOT NULL,
    taken_at     TEXT    NOT NULL,
    tz           TEXT,
    local_date   TEXT    NOT NULL,
    month_day    TEXT    NOT NULL,
    year         INTEGER NOT NULL,
    text         TEXT    NOT NULL DEFAULT '',
    place_name   TEXT    NOT NULL DEFAULT '',
    lat          REAL,
    lon          REAL,
    country_code TEXT    NOT NULL DEFAULT '',
    region       TEXT    NOT NULL DEFAULT '',
    city         TEXT    NOT NULL DEFAULT '',
    media_count  INTEGER NOT NULL DEFAULT 0,
    photo_count  INTEGER NOT NULL DEFAULT 0,
    video_count  INTEGER NOT NULL DEFAULT 0,
    vault        INTEGER NOT NULL DEFAULT 0,
    duplicate_of TEXT,
    edited       INTEGER NOT NULL DEFAULT 0,
    import_id    INTEGER,
    path         TEXT    NOT NULL,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);
CREATE INDEX idx_memories_day   ON memories(month_day, year);
CREATE INDEX idx_memories_taken ON memories(taken_at);
CREATE INDEX idx_memories_year  ON memories(year, taken_at);
CREATE INDEX idx_memories_geo   ON memories(lat, lon) WHERE lat IS NOT NULL;
CREATE INDEX idx_memories_place ON memories(country_code, city);

CREATE TABLE media (
    id          TEXT    PRIMARY KEY,
    memory_id   TEXT    NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    kind        TEXT    NOT NULL,
    file        TEXT    NOT NULL,
    mime        TEXT    NOT NULL DEFAULT '',
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    duration    REAL    NOT NULL DEFAULT 0,
    size        INTEGER NOT NULL DEFAULT 0,
    sha256      TEXT    NOT NULL,
    dhash       INTEGER,
    taken_at    TEXT,
    lat         REAL,
    lon         REAL
);
CREATE INDEX idx_media_memory ON media(memory_id, position);
CREATE INDEX idx_media_sha    ON media(sha256);
CREATE INDEX idx_media_dhash  ON media(dhash) WHERE dhash IS NOT NULL;

-- Full-text search over memories, kept in sync with triggers.
CREATE VIRTUAL TABLE memories_fts USING fts5(
    text, place_name, city, region,
    content = 'memories',
    content_rowid = 'rowid',
    tokenize = 'unicode61 remove_diacritics 2'
);
CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, text, place_name, city, region)
    VALUES (new.rowid, new.text, new.place_name, new.city, new.region);
END;
CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, text, place_name, city, region)
    VALUES ('delete', old.rowid, old.text, old.place_name, old.city, old.region);
END;
CREATE TRIGGER memories_au AFTER UPDATE OF text, place_name, city, region ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, text, place_name, city, region)
    VALUES ('delete', old.rowid, old.text, old.place_name, old.city, old.region);
    INSERT INTO memories_fts(rowid, text, place_name, city, region)
    VALUES (new.rowid, new.text, new.place_name, new.city, new.region);
END;

-- Precomputed counts per month, used by the year overview and the timeline.
CREATE TABLE month_stats (
    year        INTEGER NOT NULL,
    month       INTEGER NOT NULL,
    memories    INTEGER NOT NULL DEFAULT 0,
    photos      INTEGER NOT NULL DEFAULT 0,
    videos      INTEGER NOT NULL DEFAULT 0,
    cover_ids   TEXT    NOT NULL DEFAULT '[]',
    PRIMARY KEY (year, month)
);

CREATE TABLE conversations (
    id                TEXT    PRIMARY KEY,
    source_key        TEXT    NOT NULL UNIQUE,
    platform          TEXT    NOT NULL,
    title             TEXT    NOT NULL,
    is_group          INTEGER NOT NULL DEFAULT 0,
    participants_json TEXT    NOT NULL DEFAULT '[]',
    first_at          TEXT,
    last_at           TEXT,
    last_text         TEXT    NOT NULL DEFAULT '',
    message_count     INTEGER NOT NULL DEFAULT 0,
    vault             INTEGER NOT NULL DEFAULT 0,
    path              TEXT    NOT NULL,
    created_at        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL
);
CREATE INDEX idx_conversations_platform ON conversations(platform, last_at);

CREATE TABLE messages (
    id               INTEGER PRIMARY KEY,
    conversation_id  TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    source_key       TEXT    NOT NULL UNIQUE,
    sender           TEXT    NOT NULL DEFAULT '',
    is_self          INTEGER NOT NULL DEFAULT 0,
    sent_at          TEXT    NOT NULL,
    kind             TEXT    NOT NULL,
    text             TEXT    NOT NULL DEFAULT '',
    attachments_json TEXT    NOT NULL DEFAULT '[]'
);
CREATE INDEX idx_messages_conversation ON messages(conversation_id, sent_at);

CREATE VIRTUAL TABLE messages_fts USING fts5(
    text,
    content = 'messages',
    content_rowid = 'id',
    tokenize = 'unicode61 remove_diacritics 2'
);
CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.id, old.text);
END;
CREATE TRIGGER messages_au AFTER UPDATE OF text ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.id, old.text);
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

-- Attachments inside messages (photos, videos, voice messages, documents).
CREATE TABLE message_files (
    id              TEXT    PRIMARY KEY,
    conversation_id TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    kind            TEXT    NOT NULL,
    file            TEXT    NOT NULL,
    name            TEXT    NOT NULL DEFAULT '',
    mime            TEXT    NOT NULL DEFAULT '',
    size            INTEGER NOT NULL DEFAULT 0,
    duration        REAL    NOT NULL DEFAULT 0,
    pages           INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_message_files_conversation ON message_files(conversation_id);
