# The Take-in

The Take-in is how you set up Afterfeed and bring in your exports. It runs the first time you open Afterfeed, and you can start it again at any time with **Add source** in the menu.

Take your time with it. Requesting an export can take hours or days, and you do everything at your own pace. Afterfeed works with whatever you have imported so far.

## 1. Language and account

Choose a language and create your account. Afterfeed has one account, which you use on every device. You can turn on [two-factor authentication](/security#two-factor-authentication) later in the settings.

## 2. Choose your sources

Select the platforms you want to import now: Facebook, Instagram, X or Snapchat. You can add the others later.

## 3. For each source

Afterfeed asks whether you already have the export.

- **I have the export** goes straight to the upload.
- **Help me request it** shows step-by-step instructions for that platform, including which options to select. When the export is ready, come back and upload it.

The instructions are also in this documentation:

- [Facebook](./exports/facebook)
- [Instagram](./exports/instagram)
- [X](./exports/x)
- [Snapchat](./exports/snapchat)

## 4. Upload the export

Platforms often split a large export into several zip files. Add all of them at once, in any order. Afterfeed recognises which parts belong together and shows how many it has, for example *3 of 5 parts*. The import starts when every part is there, because a photo and its metadata can be in different parts.

Uploads continue where they stopped if the connection drops or you close the browser. For very large exports, copying the files to the import folder on your server is faster; see [Importing large exports](./large-exports).

## 5. The import

Afterfeed unpacks the archive, reads the posts, copies the photos and videos, matches each file with its date, location and description, and prepares previews. You can close the browser while it runs.

## 6. The report

When the import is done you see what came in per year, how many items were new and how many were already in Afterfeed, and any files that could not be read. Check that every year you expect is there before you close the account on the platform.

Then [keep the original export files](./keep-your-exports) somewhere other than Afterfeed.

## Importing again

You can import the same export again, or a newer export of the same account. Afterfeed adds only what is new. Changes you made in the metadata editor are kept.
