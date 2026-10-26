-- Text recognised in photos and video frames, so search finds it.
-- media.ocr_text is NULL until the media has been looked at.
ALTER TABLE media ADD COLUMN ocr_text TEXT;
ALTER TABLE memories ADD COLUMN ocr_text TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_media_ocr_pending ON media(memory_id) WHERE ocr_text IS NULL;

-- The search index gets a column for it.
DROP TRIGGER memories_ai;
DROP TRIGGER memories_ad;
DROP TRIGGER memories_au;
DROP TABLE memories_fts;
CREATE VIRTUAL TABLE memories_fts USING fts5(
    text, place_name, city, region, album, ocr_text,
    content = 'memories',
    content_rowid = 'rowid',
    tokenize = 'unicode61 remove_diacritics 2'
);
CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, text, place_name, city, region, album, ocr_text)
    VALUES (new.rowid, new.text, new.place_name, new.city, new.region, new.album, new.ocr_text);
END;
CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, text, place_name, city, region, album, ocr_text)
    VALUES ('delete', old.rowid, old.text, old.place_name, old.city, old.region, old.album, old.ocr_text);
END;
CREATE TRIGGER memories_au AFTER UPDATE OF text, place_name, city, region, album, ocr_text ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, text, place_name, city, region, album, ocr_text)
    VALUES ('delete', old.rowid, old.text, old.place_name, old.city, old.region, old.album, old.ocr_text);
    INSERT INTO memories_fts(rowid, text, place_name, city, region, album, ocr_text)
    VALUES (new.rowid, new.text, new.place_name, new.city, new.region, new.album, new.ocr_text);
END;
INSERT INTO memories_fts(memories_fts) VALUES ('rebuild');
