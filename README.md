# Boombrick

A music player for the TrimUI Brick running NextUI. It plays your own files, streams Spotify, downloads music from YouTube Music for offline listening, and shows synced lyrics in colours taken from the album art.

Early development: the UI foundation and the Home screen (with sample content) run on desktop and on the Brick. Playback comes next.

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

Unzip it at the root of the SD card and start Boombrick from Tools.

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

Hold Select and Start together to quit.

## Documentation

- [docs/design.md](docs/design.md): behaviour and architecture
- [docs/ui.md](docs/ui.md): visual design and screen layouts
- [docs/plans/](docs/plans/): implementation plans

## Licence

GPL-3.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
