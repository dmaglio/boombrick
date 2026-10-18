# Changelog

All notable changes to Boombrick. Versions match the release tags.

## [Unreleased]

### Added

- **Spotify** (Premium): sign in with a code or a QR code on your phone, then play Liked Songs and your playlists, Spotify's own mixes included, in the same queue as your music. Streams play at 96, 160 or 320 kbps (Settings → Streaming quality) and are levelled like your files. Home shows your Spotify playlists, and Settings the account, with Sign out.
- **Spotify import:** Y on the Spotify tab, or X on a playlist, downloads its songs from YouTube Music, matched by ISRC or by title, artist and length. Doubtful matches wait for you to use, swap or skip them, and a progress page follows the downloads. Songs with a copy are marked on playlist pages and play from the file, offline too (Settings → Prefer downloaded copies).
- Offline, the Spotify and YouTube tabs say so, with a button that opens Downloads.
- `THIRD-PARTY.txt` in both zips holds the third-party licences in full, next to NOTICE.

### Changed

- The status bar shows only the Wi-Fi-off icon when offline, not the word "Offline" as well.

### Fixed

- YouTube Music thumbnails missed while offline show a few seconds after the Wi-Fi is back, not half a minute later.
- A song played right after it was removed from the downloads starts downloading first, instead of after the songs already downloading.
- The download queue's list is saved again by itself when saving it failed, as on a busy card.

## [v0.2.0-beta.1]

A beta for testers: Settings doesn't offer it, and it is installed by hand.

### Added

- **CrossMix:** `Boombrick-CrossMix.zip` installs Boombrick in CrossMix's Apps, with its data in the app's folder. It keeps the Brick awake while it's open, powers off when Power is held, and updates itself from Settings.
- Betas: a version with a suffix, like this one, is published as a pre-release, which only testers install. Every version is offered the next full release.

## [v0.1.1]

### Added

- With the screen off, L1 and R1 skip back and on, and A plays or pauses.

### Fixed

- Power turns the screen back on: the Brick's backlight is woken as NextUI wakes it.

## [v0.1.0]

First release.

### Added

- **Playback:** MP3, FLAC, Opus and M4A; gapless; ReplayGain levelling; seeking; queue with shuffle and repeat, resumed after a restart.
- **Library:** Home, Albums, Artists and Songs, album and artist pages, Queue and Now Playing.
- **Album art** from tags, folder pictures and the Cover Art Archive, with colours taken from it.
- **Synced lyrics** from tags, `.lrc` files and LRCLIB.
- **YouTube Music:** search and browse; downloads saved as tagged Opus files with art and lyrics; a Downloads tab; songs that play when their download lands.
- **Quick menu** (Menu): sleep timer, screen off, Settings and Quit.
- **Settings:** download quality, storage and cache clearing, sleep timer, volume levelling.
- **Screen off:** Power turns the screen off and on, and holding it powers the Brick off.
- **Updates:** the downloader and Boombrick update from Settings, from GitHub releases.
- **Perf mode:** an empty `perf` file in the pak folder logs memory, CPU and frame times.
