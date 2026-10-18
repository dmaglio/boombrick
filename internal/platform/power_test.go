package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// panel stands in for the display driver.
type panel struct {
	level int
	fail  bool
}

func (p *panel) brightness() (int, error) { return p.level, nil }

func (p *panel) setBrightness(v int) error {
	if p.fail {
		return errors.New("driver refused")
	}
	p.level = v
	return nil
}

func TestBacklightOffAndOn(t *testing.T) {
	p := &panel{level: 128}
	b := &Backlight{dev: p}
	if err := b.Off(); err != nil || p.level != 0 {
		t.Fatalf("Off: level %d, err %v; want 0", p.level, err)
	}
	if err := b.Off(); err != nil || p.level != 0 {
		t.Fatalf("Off again: level %d, err %v", p.level, err)
	}
	if err := b.On(); err != nil || p.level != 128 {
		t.Fatalf("On: level %d, err %v; want 128 back", p.level, err)
	}
	if err := b.On(); err != nil || p.level != 128 {
		t.Fatalf("On again: level %d, err %v", p.level, err)
	}
}

func TestBacklightOnAfterADarkStart(t *testing.T) {
	p := &panel{}
	b := &Backlight{dev: p}
	b.Off()
	if b.On(); p.level != fallbackBrightness {
		t.Fatalf("level %d after On, want %d", p.level, fallbackBrightness)
	}
}

func TestBacklightOffFailing(t *testing.T) {
	p := &panel{level: 64, fail: true}
	b := &Backlight{dev: p}
	if err := b.Off(); err == nil {
		t.Fatal("Off reported no error")
	}
	p.fail = false
	if b.On(); p.level != 64 {
		t.Fatalf("On after a failed Off set the level to %d", p.level)
	}
}

func TestNoDisplayDriver(t *testing.T) {
	b := &Backlight{dev: dispDevice{path: filepath.Join(t.TempDir(), "disp")}}
	if err := b.Off(); err == nil {
		t.Fatal("Off without a display driver reported no error")
	}
}

func TestPowerOff(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "nextui_exec"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PowerOff(tmp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "nextui_exec")); !os.IsNotExist(err) {
		t.Error("nextui_exec is still there")
	}
	if _, err := os.Stat(filepath.Join(tmp, "poweroff")); err != nil {
		t.Errorf("poweroff wasn't written: %v", err)
	}
	if err := PowerOff(tmp); err != nil {
		t.Errorf("a second PowerOff failed: %v", err)
	}
}
