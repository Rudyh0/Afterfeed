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

Platforms often split a large export into several zip files. Add all of them, in any order. A photo and its date and description can be in different parts, so Afterfeed reads all parts together.

Before it copies anything, Afterfeed checks that every photo, video and attachment the export refers to is in the files you added. When files are missing, it shows how many, with examples. Add the other parts of the export, or choose **Import without them**. Importing the same export again later, with the missing parts, adds the files to the memories that were already imported.

Uploads continue where they stopped if the connection drops or you close the browser. For very large exports, copying the files to the import folder on your server is faster; see [Importing large exports](./large-exports).

## 5. The import

Afterfeed reads the posts, copies the photos and videos out of the zip files, matches each file with its date, location and description, and stores the chats. The screen shows how far it is. You can close the tab and come back later: the import continues on the server, and every screen of the app shows its progress. Previews are prepared in the background afterwards. To follow an import in detail, see [An import seems to take forever](/maintenance/troubleshooting#an-import-seems-to-take-forever).

## 6. The report

When the import is done you see what came in per year, how many items were new and how many were already in Afterfeed, and any files that could not be read. Check that every year you expect is there before you close the account on the platform.

Then [keep the original export files](./keep-your-exports) somewhere other than Afterfeed.

## Importing again

You can import the same export again, or a newer export of the same account. Afterfeed adds only what is new. Changes you made in the metadata editor are kept.

## Removing a source

To take a platform out of Afterfeed, for example to start over with a newer export, go to **Settings › Sources** and choose **Remove** next to it. Afterfeed shows what will go and asks for your password.

Removing a source deletes:

- its memories and conversations, also those in the vault;
- their photos, videos and files in your data folder;
- changes you made to them in the metadata editor;
- its import history.

The uploaded export files are kept, so you can import the source again without uploading. Tick **Also delete the uploaded export files** to remove them too. Export files you put in the import folder yourself are never touched.

This cannot be undone. Your own memories, made in Afterfeed, are not a source and stay; delete those one by one.
