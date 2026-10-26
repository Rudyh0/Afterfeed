# Search

Search looks through the text of your memories, place names, albums and messages. It also finds words that appear *in* your photos and videos, such as a sign, a screenshot or a Snapchat caption.

## What you can type

- **Words:** every word must match, and the start of a word is enough: `athe` finds Athens.
- **Dates and seasons:** `2019`, `summer 2021`, `july 2018`. These work in every language of the interface.
- **Places:** a place or city, like `Lisbon`.

You can combine them: `beach summer 2021`.

## Text in photos

After an import, Afterfeed reads the text in your photos and videos in the background, once the thumbnails are ready. While it works, the menu shows **Reading text in photos** with the number left. You can keep using Afterfeed meanwhile; new photos are searchable as soon as they have been read.

The text that was found is shown with the photo under **Text in image**. Only words the recogniser is fairly sure of are kept, so a photo without text stays empty rather than collecting noise. Handwriting and stylised fonts are often not recognised.

The text is stored in `memory.json` next to the photo, so it survives [rebuilding the index](/maintenance/commands) and moving to another server without being read again.

### Languages

The Docker image reads English, Dutch, German, French and Spanish. Fewer languages is a little faster and more accurate. Set `AFTERFEED_OCR_LANGUAGES` to the ones you need, joined with `+`:

```yaml
environment:
  - AFTERFEED_OCR_LANGUAGES=nld+eng
```

| Code | Language |
| --- | --- |
| `eng` | English |
| `nld` | Dutch |
| `deu` | German |
| `fra` | French |
| `spa` | Spanish |

Set `AFTERFEED_OCR_LANGUAGES=off` to turn reading text off. Text that was already read stays searchable.

Changing the languages does not read photos again that were already read.

### How long it takes

Reading text runs at a low priority, next to whatever you are looking at. It takes about as long as making the thumbnails did. On a small NAS a large archive can take several hours; the log shows `reading text in photos` with the number left every minute.
