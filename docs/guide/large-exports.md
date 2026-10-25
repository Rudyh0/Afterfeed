# Importing large exports

Exports with many photos and videos can be tens or hundreds of gigabytes. You can upload them in the browser, but copying them straight to your server is faster and does not depend on keeping a browser open.

## Use the import folder

1. Copy the export files to the `import` folder on your data volume. With the default setup that is `data/import` next to your `config` folder. You can use SMB, SFTP, `rsync` or your NAS file manager.
2. In the Take-in, open the upload step for the platform. Afterfeed lists the files it finds in the import folder.
3. Choose **Use these files**.

Afterfeed leaves files in the import folder untouched. Delete them yourself after the import, once you have a copy elsewhere.

## Browser uploads

Uploads in the browser are sent in parts of 16 MB. When the connection drops, or you close the tab, the upload continues where it stopped the next time you open the upload step and add the same file. Files uploaded through the browser are removed from the server after a successful import.

## Disk space

During an import Afterfeed needs free space for the photos and videos it copies out of the archive, which is roughly the size of the export, plus the export itself if you uploaded it. Previews add a few percent.
