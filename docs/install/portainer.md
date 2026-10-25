# Portainer

1. In Portainer, open your environment and go to **Stacks**.
2. Click **Add stack** and name it `afterfeed`.
3. Choose **Web editor** and paste:

   ```yaml
   services:
     afterfeed:
       image: afterfeed/afterfeed:latest
       container_name: afterfeed
       environment:
         - PUID=1000
         - PGID=1000
         - TZ=Europe/Amsterdam
       volumes:
         - /path/to/afterfeed/config:/config
         - /path/to/afterfeed/data:/data
       ports:
         - 8080:8080
       restart: unless-stopped
   ```

4. Replace `/path/to/afterfeed` with a folder on the host and set `PUID`, `PGID` and `TZ` (see [User and group](./#user-and-group)). Use full paths: Portainer stacks do not have a working folder for `./` paths.
5. Click **Deploy the stack**.
6. Open `http://<your-server>:8080` and follow the [Take-in](/guide/take-in).

## Updating

Open the stack, click **Editor**, then **Update the stack** with **Re-pull image and redeploy** turned on.
