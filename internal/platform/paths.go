// Package platform holds device integration: the firmware Boombrick runs
// under and where its data lives, the battery, network and Bluetooth state
// the status bar shows, power, the volume and the screen. Input is in a
// subpackage.
package platform

import (
	"os"
	"path/filepath"
)

// Firmware is what runs Boombrick: one of the Brick's firmwares, or a
// desktop.
type Firmware uint8

const (
	Desktop Firmware = iota
	NextUI
	CrossMix
)

func (f Firmware) String() string {
	switch f {
	case NextUI:
		return "NextUI"
	case CrossMix:
		return "CrossMix"
	}
	return "desktop"
}

// Paths are the directories Boombrick reads and writes.
type Paths struct {
	Data     string // database, caches, credentials
	Logs     string
	Music    string // scanned library root
	OnDevice bool
}

// Profile is what differs from one firmware to another: where Boombrick
// keeps its files, how it powers the device off, and the release file that
// updates it.
type Profile struct {
	Firmware Firmware
	Paths    Paths
	// ReleaseAsset is the release file self-update installs, and ReleaseDir
	// the folder in its zip that holds the app's own files; both "" on a
	// desktop.
	ReleaseAsset, ReleaseDir string
	tmp                      string // where the power-off files go: /tmp
}

// Detect works out the firmware from what its launcher passes. CrossMix's
// launch.sh, Boombrick's own, sets BOOMBRICK_PLATFORM=crossmix; NextUI
// passes USERDATA_PATH and its other paths; a desktop passes neither, and
// $BOOMBRICK_DATA (default dist/desktop/data) holds everything. dir is the
// program's folder, which on CrossMix holds Boombrick's data, as CrossMix's
// own apps keep theirs.
func Detect(dir string) Profile {
	if os.Getenv("BOOMBRICK_PLATFORM") == "crossmix" {
		if dir == "" {
			dir = "/mnt/SDCARD/Apps/Boombrick"
		}
		return Profile{Firmware: CrossMix, tmp: tmpDir(),
			Paths: Paths{
				Data:     filepath.Join(dir, "data"),
				Logs:     filepath.Join(dir, "logs"),
				Music:    filepath.Join(envOr("SDCARD_PATH", "/mnt/SDCARD"), "Music"),
				OnDevice: true,
			},
			ReleaseAsset: "Boombrick-CrossMix.zip", ReleaseDir: "Apps/Boombrick/"}
	}
	if ud := os.Getenv("USERDATA_PATH"); ud != "" {
		return Profile{Firmware: NextUI, tmp: tmpDir(),
			Paths: Paths{
				Data:     filepath.Join(ud, "Boombrick"),
				Logs:     envOr("LOGS_PATH", filepath.Join(ud, "logs")),
				Music:    filepath.Join(envOr("SDCARD_PATH", "/mnt/SDCARD"), "Music"),
				OnDevice: true,
			},
			ReleaseAsset: "Boombrick.pak.zip", ReleaseDir: "Tools/tg5040/Boombrick.pak/"}
	}
	base := envOr("BOOMBRICK_DATA", filepath.Join("dist", "desktop", "data"))
	return Profile{Firmware: Desktop, tmp: tmpDir(), Paths: Paths{Data: base, Logs: base, Music: filepath.Join(base, "Music")}}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// tmpDir is where the power-off flags, the background player's lock and
// CrossMix's keep-awake file go: /tmp, or $BOOMBRICK_TMP, as CrossMix's
// launch.sh takes it too.
func tmpDir() string { return envOr("BOOMBRICK_TMP", "/tmp") }

// PlayerLock is the lock the background player holds while it owns the queue.
func (p Profile) PlayerLock() string { return filepath.Join(p.tmp, "boombrick-player.lock") }

// StayAwake is the file that keeps CrossMix from sleeping while it exists,
// or "" under other firmwares.
func (p Profile) StayAwake() string {
	if p.Firmware != CrossMix {
		return ""
	}
	return filepath.Join(p.tmp, "stay_awake")
}
