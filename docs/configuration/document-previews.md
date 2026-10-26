# Document previews

Messages can contain documents. Afterfeed shows a preview of the first page of PDF files on its own. For Word, Excel and PowerPoint files it needs an extra service, because rendering those requires LibreOffice, which would make the Afterfeed image much larger.

Without the extra service, those documents show their name, type and size with a download button.

## Add the document service

Add [Gotenberg](https://gotenberg.dev) next to Afterfeed and tell Afterfeed where to find it:

```yaml
services:
  afterfeed:
    image: afterfeed/afterfeed:latest
    environment:
      - AFTERFEED_PREVIEW_URL=http://documents:3000
    # … the rest of your configuration

  documents:
    image: gotenberg/gotenberg:8
    container_name: afterfeed-documents
    restart: unless-stopped
```

A preview is created the first time the document is shown in a conversation and then kept in the cache. Documents that were shown before you added the service get a preview the next time they are shown.

## Safety

Documents in your messages come from other people. Afterfeed never runs macros or scripts in them. Previews are created as images by the document service, and downloading gives you the original file.
