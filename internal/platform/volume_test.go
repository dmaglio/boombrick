package platform

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// settingsFile writes a SettingsV11 prefix with the fields set, by index.
func settingsFile(t *testing.T, path string, n int, fields map[int]int32) {
	t.Helper()
	b := make([]byte, n*4)
	for i, v := range fields {
		binary.LittleEndian.PutUint32(b[i*4:], uint32(v))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNextUIVolume(t *testing.T) {
	base := func(over map[int]int32) map[int]int32 {
		m := map[int]int32{settingsVersion: 11, settingsHeadphone: 7, settingsSpeaker: 12, settingsToggled: muteNoChange}
		for k, v := range over {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name   string
		n      int
		fields map[int]int32
		want   int
		ok     bool
	}{
		{"speaker", 40, base(nil), 12, true},
		{"jack means headphones", 40, base(map[int]int32{settingsJack: 1}), 7, true},
		{"bluetooth sink means headphones", 40, base(map[int]int32{settingsSink: 1}), 7, true},
		{"muted with a volume to return to", 40, base(map[int]int32{settingsMute: 1, settingsToggled: 0}), 0, true},
		{"muted with nothing to return to", 40, base(map[int]int32{settingsMute: 1}), 12, true},
		{"just the fields read", 29, base(nil), 12, true},
		{"wrong version", 40, base(map[int]int32{settingsVersion: 10}), 0, false},
		{"short file", 28, base(nil), 0, false},
		{"above 20", 40, base(map[int]int32{settingsSpeaker: 21}), 0, false},
		{"below 0", 40, base(map[int]int32{settingsSpeaker: -1}), 0, false},
	}
	for _, c := range cases {
		p := filepath.Join(t.TempDir(), "SharedSettings")
		settingsFile(t, p, c.n, c.fields)
		if got, ok := NextUIVolume(p); got != c.want || ok != c.ok {
			t.Errorf("%s: %d, %v; want %d, %v", c.name, got, ok, c.want, c.ok)
		}
	}
	if _, ok := NextUIVolume(filepath.Join(t.TempDir(), "missing")); ok {
		t.Error("a missing file read ok")
	}
}

func TestVolumeSettingsIsNextUIs(t *testing.T) {
	if got := (Profile{Firmware: NextUI}).VolumeSettings(); got != "/dev/shm/SharedSettings" {
		t.Errorf("NextUI: %q", got)
	}
	for _, f := range []Firmware{Desktop, CrossMix} {
		if got := (Profile{Firmware: f}).VolumeSettings(); got != "" {
			t.Errorf("%v: %q", f, got)
		}
	}
}

func TestWatchVolume(t *testing.T) {
	p := filepath.Join(t.TempDir(), "SharedSettings")
	set := func(v int32) {
		settingsFile(t, p, 29, map[int]int32{settingsVersion: 11, settingsSpeaker: v, settingsToggled: muteNoChange})
	}
	set(5)
	var mu sync.Mutex
	var got []int
	heard := func() []int { mu.Lock(); defer mu.Unlock(); return append([]int(nil), got...) }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		WatchVolume(ctx, p, 5*time.Millisecond, func(l int) { mu.Lock(); got = append(got, l); mu.Unlock() })
		close(done)
	}()
	settle := func() { time.Sleep(60 * time.Millisecond) }
	settle()
	if g := heard(); len(g) != 0 {
		t.Fatalf("the baseline was reported: %v", g)
	}
	set(8)
	settle()
	if g := heard(); len(g) != 1 || g[0] != 8 {
		t.Fatalf("after a change: %v", g)
	}
	set(8)
	settle()
	if g := heard(); len(g) != 1 {
		t.Fatalf("an unchanged rewrite was reported: %v", g)
	}
	settingsFile(t, p, 10, nil) // unreadable: skipped, the last good one stays
	settle()
	set(8)
	settle()
	if g := heard(); len(g) != 1 {
		t.Fatalf("a bad reading counted as a change: %v", g)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WatchVolume didn't stop with its context")
	}
}
