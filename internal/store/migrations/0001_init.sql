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

-- One row per import. An import reads one export, which can consist of
-- several archive parts (uploaded files or files in the import folder).
CREATE TABLE imports (
    id          INTEGER PRIMARY KEY,
    platform    TEXT    NOT NULL,
    status      TEXT    NOT NULL,
    phase       TEXT    NOT NULL DEFAULT '',
    progress    REAL    NOT NULL DEFAULT 0,
    files_json  TEXT    NOT NULL DEFAULT '[]',
    force       INTEGER NOT NULL DEFAULT 0,
    check_json  TEXT,
    report_json TEXT,
    error       TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL,
    started_at  TEXT,
    finished_at TEXT
);
CREATE INDEX idx_imports_status ON imports(status, id);

-- Browser uploads. Files are written in chunks so an interrupted upload can
-- continue where it stopped. An upload is found again by name, size and
-- modification time.
CREATE TABLE uploads (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    modified   INTEGER NOT NULL DEFAULT 0,
    received   INTEGER NOT NULL DEFAULT 0,
    platform   TEXT    NOT NULL DEFAULT '',
    detected   TEXT    NOT NULL DEFAULT '',
    status     TEXT    NOT NULL,
    import_id  INTEGER,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);
CREATE INDEX idx_uploads_fingerprint ON uploads(name, size, modified);

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
    link         TEXT    NOT NULL DEFAULT '',
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
    media_key   TEXT    NOT NULL,
    kind        TEXT    NOT NULL,
    file        TEXT    NOT NULL,
    overlay     TEXT    NOT NULL DEFAULT '',
    text        TEXT    NOT NULL DEFAULT '',
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

-- Conversations and their messages. Every conversation also has a
-- conversation.json file under /data/messages, which is the source of truth.
CREATE TABLE conversations (
    id                TEXT    PRIMARY KEY,
    source_key        TEXT    NOT NULL UNIQUE,
    merge_key         TEXT    NOT NULL DEFAULT '',
    platform          TEXT    NOT NULL,
    title             TEXT    NOT NULL,
    is_group          INTEGER NOT NULL DEFAULT 0,
    participants_json TEXT    NOT NULL DEFAULT '[]',
    self_name         TEXT    NOT NULL DEFAULT '',
    first_at          INTEGER,
    last_at           INTEGER,
    last_text         TEXT    NOT NULL DEFAULT '',
    message_count     INTEGER NOT NULL DEFAULT 0,
    file_count        INTEGER NOT NULL DEFAULT 0,
    vault             INTEGER NOT NULL DEFAULT 0,
    path              TEXT    NOT NULL,
    created_at        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL
);
CREATE INDEX idx_conversations_platform ON conversations(platform, last_at);
CREATE INDEX idx_conversations_merge    ON conversations(platform, merge_key);

-- sent_at is in milliseconds since 1970, because messages within one second
-- must keep their order.
CREATE TABLE messages (
    id               INTEGER PRIMARY KEY,
    conversation_id  TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    source_key       TEXT    NOT NULL UNIQUE,
    sender           TEXT    NOT NULL DEFAULT '',
    is_self          INTEGER NOT NULL DEFAULT 0,
    sent_at          INTEGER NOT NULL,
    kind             TEXT    NOT NULL,
    text             TEXT    NOT NULL DEFAULT '',
    link             TEXT    NOT NULL DEFAULT '',
    files_json       TEXT    NOT NULL DEFAULT '[]',
    reactions_json   TEXT    NOT NULL DEFAULT '[]'
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

-- Files in messages (photos, videos, voice messages, documents).
CREATE TABLE message_files (
    id              TEXT    PRIMARY KEY,
    conversation_id TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    message_key     TEXT    NOT NULL,
    sent_at         INTEGER NOT NULL,
    kind            TEXT    NOT NULL,
    file            TEXT    NOT NULL,
    name            TEXT    NOT NULL DEFAULT '',
    mime            TEXT    NOT NULL DEFAULT '',
    size            INTEGER NOT NULL DEFAULT 0,
    width           INTEGER NOT NULL DEFAULT 0,
    height          INTEGER NOT NULL DEFAULT 0,
    duration        REAL    NOT NULL DEFAULT 0
);
CREATE INDEX idx_message_files_conversation ON message_files(conversation_id, kind, sent_at);
