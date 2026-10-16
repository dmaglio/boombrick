package library

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/dhowden/tag"
)

func taggedFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("%s is missing; run make fixtures", path)
	}
	return path
}

func TestReadTags(t *testing.T) {
	for _, c := range []struct {
		name string
		gain bool // ffmpeg writes ReplayGain into MP3, FLAC and Opus tags, not MP4's
	}{{"tagged.mp3", true}, {"tagged.flac", true}, {"tagged.opus", true}, {"tagged.m4a", false}} {
		t.Run(c.name, func(t *testing.T) {
			tags, err := ReadTags(taggedFixture(t, c.name))
			if err != nil {
				t.Fatal(err)
			}
			want := Tags{Title: "Rust", Artist: "Odalys", AlbumArtist: "Various Artists", Album: "Copper Season",
				Disc: 2, Track: 3, Year: 2019}
			if c.gain {
				want.Gain, want.Peak, want.HasGain = -6.5, 0.9, true
				want.AlbumGain, want.AlbumPeak, want.HasAlbumGain = -7.25, 0.95, true
			}
			if math.Abs(tags.Gain-want.Gain) > 0.01 || math.Abs(tags.Peak-want.Peak) > 0.01 {
				t.Errorf("gain %v peak %v, want %v %v", tags.Gain, tags.Peak, want.Gain, want.Peak)
			}
			tags.Gain, tags.Peak = want.Gain, want.Peak
			if math.Abs(tags.AlbumGain-want.AlbumGain) > 0.01 || math.Abs(tags.AlbumPeak-want.AlbumPeak) > 0.01 {
				t.Errorf("album gain %v peak %v, want %v %v", tags.AlbumGain, tags.AlbumPeak, want.AlbumGain, want.AlbumPeak)
			}
			tags.AlbumGain, tags.AlbumPeak = want.AlbumGain, want.AlbumPeak
			if tags != want {
				t.Fatalf("ReadTags = %+v\nwant        %+v", tags, want)
			}
		})
	}
}

func TestReplayGainFromR128(t *testing.T) {
	// Opus R128 gains are Q7.8 dB against -23 LUFS, 5 dB below ReplayGain's
	// reference: -1280 is -5 dB, which is 0 dB in ReplayGain terms.
	gain, _, ok, _, _, _ := replayGains(map[string]interface{}{"r128_track_gain": "-1280"})
	if !ok || math.Abs(gain) > 1e-9 {
		t.Fatalf("R128 -1280 = %v dB (ok %v), want 0", gain, ok)
	}
}

func TestAlbumGainFromR128(t *testing.T) {
	_, _, _, gain, _, ok := replayGains(map[string]interface{}{"r128_album_gain": "-1792"})
	if !ok || math.Abs(gain-(-2)) > 1e-9 {
		t.Fatalf("R128 album -1792 = %v dB (ok %v), want -2", gain, ok)
	}
}

func TestRawText(t *testing.T) {
	raw := map[string]interface{}{
		"TXXX":                  &tag.Comm{Description: "REPLAYGAIN_TRACK_GAIN", Text: "-6.50 dB\x00"},
		"albumartist":           "  Odalys ",
		"album artist":          "Other",
		"replaygain_track_peak": "0.9",
	}
	if got := rawText(raw, "replaygain_track_gain"); got != "-6.50 dB" {
		t.Errorf("TXXX gain = %q", got)
	}
	if got := rawText(raw, "album_artist", "album artist", "albumartist"); got != "Other" {
		t.Errorf("first key present wins: got %q", got)
	}
	if got := rawText(raw, "albumartist"); got != "Odalys" {
		t.Errorf("plain value = %q, want it trimmed", got)
	}
	if got := rawText(raw, "missing"); got != "" {
		t.Errorf("missing = %q", got)
	}
}

func TestReplayGainValues(t *testing.T) {
	for _, c := range []struct {
		raw  map[string]interface{}
		gain float64
		ok   bool
	}{
		{map[string]interface{}{"replaygain_track_gain": "-6.50 db"}, -6.5, true},
		{map[string]interface{}{"replaygain_track_gain": "+3.2"}, 3.2, true},
		{map[string]interface{}{"replaygain_track_gain": "Inf"}, 0, false},
		{map[string]interface{}{"replaygain_track_gain": "NaN dB"}, 0, false},
		{map[string]interface{}{"replaygain_track_gain": "-400 dB"}, 0, false},
		{map[string]interface{}{"replaygain_track_gain": "loud", "r128_track_gain": "-1280"}, 0, true},
		{map[string]interface{}{"replaygain_track_gain": "-2 dB", "r128_track_gain": "-1280"}, -2, true},
	} {
		gain, _, ok, _, _, _ := replayGains(c.raw)
		if ok != c.ok || math.Abs(gain-c.gain) > 1e-9 {
			t.Errorf("%v = %v, %v; want %v, %v", c.raw, gain, ok, c.gain, c.ok)
		}
	}
	if _, peak, _, _, _, _ := replayGains(map[string]interface{}{"replaygain_track_peak": "NaN"}); peak != 0 {
		t.Errorf("NaN peak = %v", peak)
	}
}

// MP4 freeform atoms (----:com.apple.iTunes:name) arrive keyed by the name
// alone, the mean being dropped by the tag package.
func TestReplayGainMP4Key(t *testing.T) {
	gain, peak, ok, _, _, _ := replayGains(map[string]interface{}{
		"REPLAYGAIN_TRACK_GAIN": "-6.50 dB", "REPLAYGAIN_TRACK_PEAK": "0.9"})
	if !ok || gain != -6.5 || peak != 0.9 {
		t.Fatalf("got %v %v %v", gain, peak, ok)
	}
}

func TestYear(t *testing.T) {
	for _, tc := range []struct {
		y    int
		raw  map[string]interface{}
		want int
	}{
		{2019, map[string]interface{}{"date": "2019"}, 2019},
		{1, map[string]interface{}{"date": "20190514"}, 2019},
		{20190514, map[string]interface{}{"TDRC": "20190514"}, 2019},
		{0, map[string]interface{}{"tyer": "2007-05"}, 2007},
		{1, map[string]interface{}{"date": "May 2019"}, 0},
		{1, map[string]interface{}{"date": "0001-01-01"}, 0},
		{0, map[string]interface{}{}, 0},
	} {
		if got := year(tc.y, tc.raw); got != tc.want {
			t.Errorf("year(%d, %v) = %d, want %d", tc.y, tc.raw, got, tc.want)
		}
	}
}

func TestReadTagsSurvivesDamagedTags(t *testing.T) {
	cases := map[string][]byte{
		// An ID3 header and a frame claiming far more than the file holds.
		"id3": append([]byte("ID3\x03\x00\x00\x00\x00\x01\x00TIT2\x00\x00\x10\x00\x00\x00"), 0, 'a'),
		// An MP4 moov, udta, meta and ilst nested with sizes past the end.
		"mp4": []byte("\x00\x00\x00\x18ftypM4A \x00\x00\x00\x00M4A \x00\x00\x7f\x00moov\x00\x00\x7f\x00udta\x00\x00\x7f\x00meta\x00\x00\x00\x00\x00\x00\x7f\x00ilst\x00\x00\x7f\x00\xa9nam\x00\x00\x7f\x00data"),
	}
	for name, data := range cases {
		path := filepath.Join(t.TempDir(), "x.mp3")
		os.WriteFile(path, data, 0o644)
		ReadTags(path) // must not panic
		t.Log(name, "ok")
	}
}
