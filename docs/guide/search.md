# Search

Search looks through the text of your memories, place names, albums and messages. It also finds words that appear *in* your photos and videos, such as a sign, a number plate, a screenshot or a Snapchat caption.

## What you can type

- **Words:** every word must match, and the start of a word is enough: `athe` finds Athens.
- **Dates and seasons:** `2019`, `summer 2021`, `july 2018`. These work in every language of the interface.
- **Places:** a place or city, like `Lisbon`.

You can combine them: `beach summer 2021`.

## Text in photos

After an import, Afterfeed reads the text in your photos and videos in the background, once the thumbnails are ready. While it works, the menu shows **Reading text in photos** with the number left. You can keep using Afterfeed meanwhile; new photos are searchable as soon as they have been read.

Reading happens in two steps. A text detection model first finds where the lines of text are, also small or tilted text in a scene, like a number plate on a car or a street sign. Tesseract then reads those lines. Both run inside Afterfeed; nothing leaves your server.

The text that was found is shown with the photo under **Text in image**. Only words the reader is fairly sure of are kept, so a photo without text stays empty rather than collecting noise. Handwriting and stylised fonts are often not recognised.

### Number plates and codes

Codes written in parts are found however you type them. A number plate read as `GX-707-Z` is found with `GX-707-Z`, `gx707z` or the start, `gx707`. The same goes for postcodes such as `9351 AB`.

### Stored with the photo

The text is stored in `memory.json` next to the photo, so it survives [rebuilding the index](/maintenance/commands) and moving to another server without being read again.

When an update of Afterfeed reads text better, photos that were read before are read again in the background. Their earlier text stays searchable until then.

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

Reading text runs at a low priority, next to whatever you are looking at, on all but one processor core. Expect about a second per photo per core on a desktop processor and a few seconds on a small NAS: a few thousand photos take under an hour, a very large archive can take a night. The log shows `reading text in photos` with the number left every minute.
