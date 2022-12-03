# Kupo

Utility program updating a Twitch's channel information based on the VLC's playing track.

## Setup

### Twitch

Create a new application (or use an existing one) in the [Twitch Developer Console](https://dev.twitch.tv/console/apps) with a **Redirect URI** set to `http://localhost:21825`.

Copy the **Client ID** and **Client Secret** to the confgiration file.

You can also customize the stream title by modifying the `title` in the configuration file.

### VLC

If you don't have it installed, download it from the [official website](https://www.videolan.org/vlc).

## Usage

During the first launch, it'll invite you to authorize access to your Twitch account for updating your channel information based on the playing track.

A VLC window should open afterwards, where you can fill your playlist.

The program uses both `Title` and `Album` metadata, so make sure they are filled and correct.

## License

Copyright (c) 2022-present Alexandre Breteau

This software is released under the terms of the MIT License.
See the [LICENSE](LICENSE) file for further information.
