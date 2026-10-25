# QNAP

These steps use **Container Station 3**.

## 1. Create the folders

In **File Station**, create a folder `afterfeed` with two subfolders, `config` and `data`, for example in the `Container` shared folder.

## 2. Find your user and group ID

Enable SSH under **Control Panel › Network & File Services › Telnet / SSH**, connect, and run:

```sh
id your-username
```

Use the `uid` as `PUID` and the `gid` as `PGID`. Avoid running Afterfeed as `admin`.

## 3. Create the application

1. Open **Container Station** and go to **Applications**.
2. Click **Create**, name the application `afterfeed`, and paste:

   ```yaml
   services:
     afterfeed:
       image: afterfeed/afterfeed:latest
       container_name: afterfeed
       environment:
         - PUID=1000
         - PGID=100
         - TZ=Europe/Amsterdam
       volumes:
         - /share/Container/afterfeed/config:/config
         - /share/Container/afterfeed/data:/data
       ports:
         - 8080:8080
       restart: unless-stopped
   ```

3. Change `PUID`, `PGID`, `TZ` and the paths to your values.
4. Click **Validate**, then **Create**.
5. Open `http://<your-nas>:8080` and follow the [Take-in](/guide/take-in).

## Large exports

Copy large export files over SMB to `afterfeed/data/import`. The Take-in finds them there.

## Updating

In **Applications**, open `afterfeed`, choose **Recreate** and turn on pulling the latest image.
