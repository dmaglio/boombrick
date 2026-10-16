package platform

import "testing"

func TestDetectPathsOnDevice(t *testing.T) {
	t.Setenv("USERDATA_PATH", "/mnt/SDCARD/.userdata/tg5040")
	t.Setenv("SDCARD_PATH", "/mnt/SDCARD")
	t.Setenv("LOGS_PATH", "/mnt/SDCARD/.userdata/tg5040/logs")
	want := Paths{
		Data:     "/mnt/SDCARD/.userdata/tg5040/Boombrick",
		Logs:     "/mnt/SDCARD/.userdata/tg5040/logs",
		Music:    "/mnt/SDCARD/Music",
		OnDevice: true,
	}
	if got := DetectPaths(); got != want {
		t.Fatalf("DetectPaths() = %+v, want %+v", got, want)
	}
}

func TestDetectPathsOnDesktop(t *testing.T) {
	t.Setenv("USERDATA_PATH", "")
	t.Setenv("BOOMBRICK_DATA", "/tmp/bb")
	want := Paths{Data: "/tmp/bb", Logs: "/tmp/bb", Music: "/tmp/bb/Music"}
	if got := DetectPaths(); got != want {
		t.Fatalf("DetectPaths() = %+v, want %+v", got, want)
	}
}
