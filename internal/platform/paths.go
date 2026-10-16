// Package platform holds device integration: where data lives, and (in
// subpackages) input, power and connectivity.
package platform

import (
	"os"
	"path/filepath"
)

// Paths are the directories Boombrick reads and writes.
type Paths struct {
	Data     string // database, caches, credentials
	Logs     string
	Music    string // scanned library root
	OnDevice bool
}

// DetectPaths uses the variables NextUI passes to launch.sh when present, and
// $BOOMBRICK_DATA (default dist/desktop/data) on a desktop.
func DetectPaths() Paths {
	if ud := os.Getenv("USERDATA_PATH"); ud != "" {
		sd := envOr("SDCARD_PATH", "/mnt/SDCARD")
		return Paths{
			Data:     filepath.Join(ud, "Boombrick"),
			Logs:     envOr("LOGS_PATH", filepath.Join(ud, "logs")),
			Music:    filepath.Join(sd, "Music"),
			OnDevice: true,
		}
	}
	base := envOr("BOOMBRICK_DATA", filepath.Join("dist", "desktop", "data"))
	return Paths{Data: base, Logs: base, Music: filepath.Join(base, "Music")}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
