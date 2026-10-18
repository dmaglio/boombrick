package platform

import "testing"

func TestDetectNextUI(t *testing.T) {
	t.Setenv("BOOMBRICK_TMP", "")
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
	t.Setenv("BOOMBRICK_TMP", "")
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
	t.Setenv("BOOMBRICK_TMP", "")
	t.Setenv("BOOMBRICK_PLATFORM", "")
	t.Setenv("USERDATA_PATH", "")
	t.Setenv("BOOMBRICK_DATA", "/tmp/bb")
	want := Profile{Firmware: Desktop, tmp: "/tmp", Paths: Paths{Data: "/tmp/bb", Logs: "/tmp/bb", Music: "/tmp/bb/Music"}}
	if got := Detect("/home/me/boombrick/dist/desktop"); got != want {
		t.Fatalf("Detect() = %+v, want %+v", got, want)
	}
}

// The background player's lock and CrossMix's keep-awake file are in /tmp,
// or $BOOMBRICK_TMP, as tests and CrossMix's launch.sh use it.
func TestTempFiles(t *testing.T) {
	t.Setenv("USERDATA_PATH", "")
	t.Setenv("BOOMBRICK_PLATFORM", "crossmix")
	t.Setenv("BOOMBRICK_TMP", "")
	p := Detect("/mnt/SDCARD/Apps/Boombrick")
	if p.PlayerLock() != "/tmp/boombrick-player.lock" || p.StayAwake() != "/tmp/stay_awake" {
		t.Fatalf("CrossMix: %q, %q", p.PlayerLock(), p.StayAwake())
	}
	t.Setenv("USERDATA_PATH", "/mnt/SDCARD/.userdata/tg5040")
	t.Setenv("BOOMBRICK_PLATFORM", "")
	p = Detect("/mnt/SDCARD/Tools/tg5040/Boombrick.pak")
	if p.PlayerLock() != "/tmp/boombrick-player.lock" || p.StayAwake() != "" {
		t.Fatalf("NextUI: %q, %q", p.PlayerLock(), p.StayAwake())
	}
	t.Setenv("USERDATA_PATH", "")
	t.Setenv("BOOMBRICK_TMP", "/run/bb")
	p = Detect("")
	if p.PlayerLock() != "/run/bb/boombrick-player.lock" || p.StayAwake() != "" {
		t.Fatalf("desktop: %q, %q", p.PlayerLock(), p.StayAwake())
	}
}
