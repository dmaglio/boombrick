package platform

import (
	"os"
	"path/filepath"
	"syscall"
)

// Backlight turns the panel's light off and back on, as NextUI does on the
// Brick: by setting the display driver's brightness to 0, having read it
// first so that it can be put back.
type Backlight struct {
	dev   display
	saved int
	off   bool
}

// display is the display driver's brightness control.
type display interface {
	brightness() (int, error)
	setBrightness(v int) error
}

// fallbackBrightness is put back when the brightness read before turning the
// light off was already 0: NextUI's level 6 of 10 on the Brick.
const fallbackBrightness = 96

// NewBacklight returns the Brick's backlight. On a desktop there is no
// /dev/disp, and Off fails.
func NewBacklight() *Backlight { return &Backlight{dev: dispDevice{path: "/dev/disp"}} }

// Off turns the light off, remembering how bright it was.
func (b *Backlight) Off() error {
	if b.off {
		return nil
	}
	v, err := b.dev.brightness()
	if err != nil {
		return err
	}
	if err := b.dev.setBrightness(0); err != nil {
		return err
	}
	if v <= 0 {
		v = fallbackBrightness
	}
	b.saved, b.off = v, true
	return nil
}

// On puts the light back as it was before Off.
func (b *Backlight) On() error {
	if !b.off {
		return nil
	}
	if err := b.dev.setBrightness(b.saved); err != nil {
		return err
	}
	b.off = false
	return nil
}

// PowerOff asks NextUI to shut the device down once Boombrick quits, as its
// own apps do when Power is held: without nextui_exec its launcher stops
// starting the menu again, and with poweroff it shuts down. tmp is /tmp on
// the device.
func PowerOff(tmp string) error {
	if err := os.Remove(filepath.Join(tmp, "nextui_exec")); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.Create(filepath.Join(tmp, "poweroff"))
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	syscall.Sync()
	return nil
}
