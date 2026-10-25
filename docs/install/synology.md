# Synology

These steps use **Container Manager** on DSM 7.2 or newer.

## 1. Create the folders

1. Open **File Station**.
2. In the `docker` shared folder (create it if it does not exist), create a folder `afterfeed` with two subfolders: `config` and `data`.

To keep your memories on another volume or shared folder, create the `data` folder there instead.

## 2. Find your user and group ID

Afterfeed should run as your own DSM user. To find its IDs, enable SSH under **Control Panel › Terminal & SNMP**, connect, and run:

```sh
id your-username
# uid=1026(your-username) gid=100(users) ...
```

On most Synology systems the first user has `uid=1026` and the `users` group has `gid=100`. You can turn SSH off again afterwards.

## 3. Create the project

1. Open **Container Manager** and go to **Project**.
2. Click **Create**.
3. Name the project `afterfeed` and set the path to `/docker/afterfeed`.
4. Choose **Create docker-compose.yml** and paste:

   ```yaml
   services:
     afterfeed:
       image: afterfeed/afterfeed:latest
       container_name: afterfeed
       environment:
         - PUID=1026
         - PGID=100
         - TZ=Europe/Amsterdam
       volumes:
         - /volume1/docker/afterfeed/config:/config
         - /volume1/docker/afterfeed/data:/data
       ports:
         - 8080:8080
       restart: unless-stopped
   ```

5. Change `PUID`, `PGID` and `TZ` to your values, and the paths if your folders are on another volume.
6. Click **Next**, skip the web portal settings, and click **Done**. Container Manager downloads the image and starts Afterfeed.
7. Open `http://<your-nas>:8080` and follow the [Take-in](/guide/take-in).

## Large exports

Uploading tens of gigabytes through the browser works, but copying them to the import folder is faster. Copy the export files with File Station or over SMB to `docker/afterfeed/data/import`. The Take-in finds them there.

## Updating

In **Container Manager › Project**, select `afterfeed`, choose **Action › Stop**, then **Action › Build**. Container Manager pulls the newest image and starts the project again.
