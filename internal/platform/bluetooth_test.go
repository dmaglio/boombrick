package platform

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// device is a BlueZ device object as GetManagedObjects lists it.
func device(alias, name, icon string, connected bool) map[string]map[string]dbus.Variant {
	props := map[string]dbus.Variant{
		"Connected": dbus.MakeVariant(connected),
		"Icon":      dbus.MakeVariant(icon),
	}
	if alias != "" {
		props["Alias"] = dbus.MakeVariant(alias)
	}
	if name != "" {
		props["Name"] = dbus.MakeVariant(name)
	}
	return map[string]map[string]dbus.Variant{"org.bluez.Device1": props}
}

func TestConnectedAudio(t *testing.T) {
	objects := map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
		"/org/bluez/hci0":                    {"org.bluez.Adapter1": {}},
		"/org/bluez/hci0/dev_11_22_33_44_55": device("Pixel", "Pixel 9", "phone", true),
		"/org/bluez/hci0/dev_AA_BB_CC_DD_EE": device("Kite Buds", "KB-200", "audio-headset", true),
		"/org/bluez/hci0/dev_FF_EE_DD_CC_BB": device("Old Speaker", "", "audio-card", false),
	}
	if got := connectedAudio(objects); got != "Kite Buds" {
		t.Fatalf("connectedAudio = %q, want Kite Buds", got)
	}
	// Without an alias, the device's own name.
	objects["/org/bluez/hci0/dev_AA_BB_CC_DD_EE"] = device("", "KB-200", "audio-headphones", true)
	if got := connectedAudio(objects); got != "KB-200" {
		t.Fatalf("connectedAudio = %q, want KB-200", got)
	}
	// A phone connected alone isn't where the music goes.
	delete(objects, "/org/bluez/hci0/dev_AA_BB_CC_DD_EE")
	if got := connectedAudio(objects); got != "" {
		t.Fatalf("connectedAudio = %q with no audio device connected", got)
	}
}

// Without a system bus, or with no BlueZ on it, there is nothing to show, and
// it is asked again only after a while.
func TestNoBluetoothService(t *testing.T) {
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+t.TempDir()+"/no-bus")
	b := &Bluetooth{}
	defer b.Close()
	start := time.Now()
	if got := b.Connected(); got != "" {
		t.Fatalf("Connected = %q without a bus", got)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v to find no bus", d)
	}
	if b.retry.Before(time.Now().Add(30 * time.Second)) {
		t.Fatalf("asks again at %v, want a while off", b.retry)
	}
}
