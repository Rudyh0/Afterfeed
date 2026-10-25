# Map

The map needs map tiles: the background images of land, water and roads. Afterfeed does not include a world map, because a detailed one is very large. Choose a source under **Settings › Map**.

## External map server

Afterfeed loads the map from a map server on the internet. The map server sees which area you are looking at and your IP address. It never sees your memories: the photos and their locations stay on your server.

By default Afterfeed uses [OpenFreeMap](https://openfreemap.org), which needs no account or key. You can enter any other [MapLibre](https://maplibre.org) style URL instead.

## Local map files

To keep everything on your own server, serve the tiles yourself. A common setup is a [Protomaps](https://protomaps.com) PMTiles file for your region, served by a tile server container, with a MapLibre style that points to it. Enter that style URL in the settings.

## Off

Turn the map off entirely. Locations are still used for search and shown on each memory.

## Place names

Afterfeed turns coordinates into country, region and city names on your own server, using place data built into the image. Searching for a country works in every app language: *Greece*, *Griekenland*, *Grèce*, *Griechenland* and *Grecia* find the same memories.
