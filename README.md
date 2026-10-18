# Boombrick

A music player for the TrimUI Brick running NextUI. It plays your own files, streams Spotify, downloads music from YouTube Music for offline listening, and shows synced lyrics in colours taken from the album art.

Early development. Playing your own files, synced lyrics, YouTube Music search and downloads, and Settings work on desktop and on the Brick; Spotify and playlist import are still to come.

## Building

Development happens on Linux. On Windows, use WSL with Ubuntu 24.04.

```
bash scripts/setup-wsl.sh   # once: Go, SDL2 headers, build tools
make test
make run                    # desktop window at 1024x768
```

Device build (needs Docker):

```
make pak                    # dist/Boombrick.pak.zip
```

Unzip it at the root of the SD card and start Boombrick from Tools. Releases carry the same zip.

An empty file named `perf` in the pak's folder (`BOOMBRICK_PERF=1` on a desktop) turns on perf mode, which logs launch times and, every 10 s, memory, CPU and frame times to `Boombrick.txt`.

## Releases

Pushing a tag like `v0.5.0` builds the pak with that version and publishes it, with its `SHA256SUMS`, as a GitHub release. Settings on the Brick offers the latest release as an update; it is installed the next time Boombrick starts.

## Desktop controls

| Brick | Keyboard |
|---|---|
| D-pad | Arrow keys |
| A, B, X, Y | K, J, I, U |
| L1, R1 | Q, E |
| L2, R2 | 1, 3 |
| Start, Select | Return, Tab |
| Menu | Escape |
| Power | P |
| Volume down, up | -, = |

Menu opens the quick menu, which has Quit. P turns the screen off and on; holding it for a second quits, as holding Power powers the Brick off. With the screen off, Q and E (L1, R1) skip back and on, and K (A) plays or pauses.

## Licence

GPL-3.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
