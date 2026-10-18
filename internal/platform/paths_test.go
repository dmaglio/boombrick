package platform

import "testing"

func TestDetectNextUI(t *testing.T) {
	t.Setenv("BOOMBRICK_PLATFORM", "")
	t.Setenv("USERDATA_PATH", "/mnt/SDCARD/.userdata/tg5040")
	t.Setenv("SDCARD_PATH", "/mnt/SDCARD")
	t.Setenv("LOGS_PATH", "/mnt/SDCARD/.userdata/tg5040/logs")
	want := Profile{Firmware: NextUI, tmp: "/tmp",
		Paths: Paths{
			Data:     "/mnt/SDCARD/.userdata/tg5040/Boombrick",
			Logs:     "/mnt/SDCARD/.userdata/tg5040/logs",
			Music:    "/mnt/SDCARD/Music",
			OnDevice: true,
		},
		ReleaseAsset: "Boombrick.pak.zip", ReleaseDir: "Tools/tg5040/Boombrick.pak/"}
	if got := Detect("/mnt/SDCARD/Tools/tg5040/Boombrick.pak"); got != want {
		t.Fatalf("Detect() = %+v, want %+v", got, want)
	}
}

// CrossMix passes no paths: Boombrick keeps its data in its app folder, as
// CrossMix's own apps do.
func TestDetectCrossMix(t *testing.T) {
	t.Setenv("BOOMBRICK_PLATFORM", "crossmix")
	t.Setenv("USERDATA_PATH", "")
	t.Setenv("SDCARD_PATH", "")
	want := Profile{Firmware: CrossMix, tmp: "/tmp",
		Paths: Paths{
			Data:     "/mnt/SDCARD/Apps/Boombrick/data",
			Logs:     "/mnt/SDCARD/Apps/Boombrick/logs",
			Music:    "/mnt/SDCARD/Music",
			OnDevice: true,
		},
		ReleaseAsset: "Boombrick-CrossMix.zip", ReleaseDir: "Apps/Boombrick/"}
	if got := Detect("/mnt/SDCARD/Apps/Boombrick"); got != want {
		t.Fatalf("Detect() = %+v, want %+v", got, want)
	}
	if got := Detect(""); got != want {
		t.Fatalf("Detect without the program's folder = %+v, want %+v", got, want)
	}
	if s := want.Firmware.String(); s != "CrossMix" {
		t.Fatalf("logged as %q", s)
	}
}

func TestDetectDesktop(t *testing.T) {
	t.Setenv("BOOMBRICK_PLATFORM", "")
	t.Setenv("USERDATA_PATH", "")
	t.Setenv("BOOMBRICK_DATA", "/tmp/bb")
	want := Profile{Firmware: Desktop, tmp: "/tmp", Paths: Paths{Data: "/tmp/bb", Logs: "/tmp/bb", Music: "/tmp/bb/Music"}}
	if got := Detect("/home/me/boombrick/dist/desktop"); got != want {
		t.Fatalf("Detect() = %+v, want %+v", got, want)
	}
}
