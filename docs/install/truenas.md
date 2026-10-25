# TrueNAS

These steps are for TrueNAS Community Edition (formerly SCALE) 24.10 or newer, where apps run on Docker.

## 1. Create the datasets

1. Go to **Datasets** and create a dataset for Afterfeed, for example `tank/apps/afterfeed`.
2. Inside it, create `config` and `data`. Put `data` on a larger pool if you prefer.
3. Give the `apps` user (`568`) read and write access to both, or use your own user.

## 2. Install with YAML

1. Go to **Apps** and click **Discover Apps**.
2. Open the menu next to **Custom App** and choose **Install via YAML**.
3. Name the app `afterfeed` and paste:

   ```yaml
   services:
     afterfeed:
       image: afterfeed/afterfeed:latest
       container_name: afterfeed
       environment:
         - PUID=568
         - PGID=568
         - TZ=Europe/Amsterdam
       volumes:
         - /mnt/tank/apps/afterfeed/config:/config
         - /mnt/tank/apps/afterfeed/data:/data
       ports:
         - 8080:8080
       restart: unless-stopped
   ```

4. Change the paths to your datasets and `TZ` to your time zone.
5. Click **Save**.
6. Open `http://<your-nas>:8080` and follow the [Take-in](/guide/take-in).

## Updating

In **Apps › Installed**, select `afterfeed` and choose **Update** when an update is shown, or edit the YAML and save it to pull the latest image.
