package platform

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// Bluetooth asks BlueZ, over the system D-Bus, which audio device is
// connected, for the status bar. The firmware's own settings pair and connect
// devices; Boombrick only reads. It is called from the status reader's
// goroutine, never the UI thread.
type Bluetooth struct {
	mu    sync.Mutex
	conn  *dbus.Conn
	retry time.Time // when to try the bus again after it failed
}

// retryBus is how long a missing bus or BlueZ is left before asking again.
const retryBus = time.Minute

// Connected is the name of the connected Bluetooth audio device, or "" when
// none is connected or BlueZ isn't there to ask.
func (b *Bluetooth) Connected() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Now().Before(b.retry) {
		return ""
	}
	if b.conn == nil {
		conn, err := dbus.ConnectSystemBus()
		if err != nil {
			b.retry = time.Now().Add(retryBus)
			return ""
		}
		b.conn = conn
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	err := b.conn.Object("org.bluez", "/").CallWithContext(ctx, "org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objects)
	if err != nil {
		// BlueZ isn't running, as with Bluetooth off, or the bus went away.
		b.conn.Close()
		b.conn, b.retry = nil, time.Now().Add(retryBus)
		return ""
	}
	return connectedAudio(objects)
}

// Close lets go of the bus.
func (b *Bluetooth) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
}

// connectedAudio picks the connected audio device from BlueZ's objects:
// headphones, a headset or a speaker, by the icon BlueZ gives it, in path
// order. Its alias is the name the user gave it, or its own.
func connectedAudio(objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant) string {
	paths := make([]dbus.ObjectPath, 0, len(objects))
	for p := range objects {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		dev, ok := objects[p]["org.bluez.Device1"]
		if !ok {
			continue
		}
		connected, _ := dev["Connected"].Value().(bool)
		icon, _ := dev["Icon"].Value().(string)
		if !connected || !strings.HasPrefix(icon, "audio-") {
			continue
		}
		for _, key := range []string{"Alias", "Name"} {
			if name, _ := dev[key].Value().(string); name != "" {
				return name
			}
		}
	}
	return ""
}
