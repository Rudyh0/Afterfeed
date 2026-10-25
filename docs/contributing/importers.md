# Importers

Each platform has its own importer in `internal/importer/<platform>/`. Platforms change their export formats without notice, so importers are the part of Afterfeed that needs the most maintenance.

## How an import works

1. **Collect the parts.** The import framework groups the archive files of one export, checks that every part is there, and opens them as one combined file tree. A file can be referenced from one part and stored in another.
2. **Read.** The importer walks the tree and produces memories, conversations and messages, with references to media files in the tree.
3. **Store.** The framework copies the media into the data volume, writes `memory.json` and `conversation.json`, and adds everything to the index.
4. **Prepare.** Thumbnails, previews and waveforms are created in the cache.

An importer only implements step 2. It never writes files itself.

## Rules

- **Stable keys.** Every memory and message needs a source key that stays the same when the same export is imported again, or when a newer export of the same account is imported. Base it on the platform's own identifiers or on the timestamp and content, never on file positions.
- **No assumptions about language.** Exports contain text in the language of the account. Never match on translated labels; use the structure and identifiers.
- **Fix text encoding.** Meta exports store text as Latin-1 bytes in UTF-8 strings. Use the shared helper to repair it.
- **Keep going.** A file that cannot be read is reported, not fatal.

## Test fixtures

Tests build small fake exports from JSON in `testdata/`. Never commit real exports. When reporting a changed format, replace every name, text and media file with placeholders, and keep only the structure.
