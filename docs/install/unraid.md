# Unraid

## Add the container

1. Go to the **Docker** tab and click **Add Container**.
2. Fill in:

   | Field | Value |
   | --- | --- |
   | Name | `afterfeed` |
   | Repository | `afterfeed/afterfeed:latest` |
   | Network Type | `Bridge` |
   | WebUI | `http://[IP]:[PORT:8080]` |

3. Add these with **Add another Path, Port, Variable, Label or Device**:

   | Type | Name | Container | Host |
   | --- | --- | --- | --- |
   | Port | Web UI | `8080` | `8080` |
   | Path | Config | `/config` | `/mnt/user/appdata/afterfeed` |
   | Path | Data | `/data` | `/mnt/user/afterfeed` |
   | Variable | PUID | `PUID` | `99` |
   | Variable | PGID | `PGID` | `100` |
   | Variable | TZ | `TZ` | `Europe/Amsterdam` |

   `99:100` is the usual `nobody:users` combination on Unraid. Create a share named `afterfeed` first, or point Data to an existing share.

4. Click **Apply**.
5. Open the WebUI and follow the [Take-in](/guide/take-in).

## Large exports

Copy large export files over SMB to the `import` folder in your Data share. The Take-in finds them there.

## Updating

On the **Docker** tab, click **Check for Updates**, then **apply update** next to Afterfeed.
