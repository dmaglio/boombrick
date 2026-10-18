package platform

import (
	"context"
	"encoding/binary"
	"os"
	"time"
)

// NextUI keeps its settings in POSIX shared memory, the file
// /dev/shm/SharedSettings: a struct of little-endian int32s, SettingsV11. These
// are the fields the volume is read from, by index.
const (
	settingsVersion   = 0
	settingsHeadphone = 3
	settingsSpeaker   = 4
	settingsMute      = 5
	settingsToggled   = 14
	settingsJack      = 27
	settingsSink      = 28
	settingsFields    = 29 // how many int32s are read

	settingsV11        = 11
	muteNoChange       = -69 // toggled_volume when nothing was muted from
	volumeLevels       = 20
	volumeSettingsFile = "/dev/shm/SharedSettings"
)

// VolumeSettings is the file NextUI keeps its volume in, or "" under other
// firmwares.
func (p Profile) VolumeSettings() string {
	if p.Firmware != NextUI {
		return ""
	}
	return volumeSettingsFile
}

// NextUIVolume reads NextUI's volume, 0 to 20, from its settings file as
// NextUI's GetVolume does. ok is false when the file can't be read, isn't
// version 11 or holds a level out of range.
func NextUIVolume(path string) (level int, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	var raw [settingsFields * 4]byte
	if n, _ := f.ReadAt(raw[:], 0); n < len(raw) {
		return 0, false
	}
	at := func(i int) int { return int(int32(binary.LittleEndian.Uint32(raw[i*4:]))) }
	if at(settingsVersion) != settingsV11 {
		return 0, false
	}
	switch {
	case at(settingsMute) != 0 && at(settingsToggled) != muteNoChange:
		level = at(settingsToggled)
	case at(settingsJack) != 0 || at(settingsSink) != 0:
		level = at(settingsHeadphone)
	default:
		level = at(settingsSpeaker)
	}
	if level < 0 || level > volumeLevels {
		return 0, false
	}
	return level, true
}

// WatchVolume reads the volume from path every interval until ctx ends and
// calls changed with each level that differs from the last one read. The first
// reading is the starting point and is not reported; readings that fail are
// skipped.
func WatchVolume(ctx context.Context, path string, every time.Duration, changed func(level int)) {
	t := time.NewTicker(every)
	defer t.Stop()
	last, have := 0, false
	for {
		if v, ok := NextUIVolume(path); ok {
			if have && v != last {
				changed(v)
			}
			last, have = v, true
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
