-- Which reader looked at the text in a medium. Media read by an older
-- reader are read again by a better one; their old text stays searchable
-- meanwhile.
ALTER TABLE media ADD COLUMN ocr_version INTEGER NOT NULL DEFAULT 0;
DROP INDEX idx_media_ocr_pending;
