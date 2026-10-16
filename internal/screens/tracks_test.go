package screens

import (
	"testing"
	"time"

	"github.com/ujjwal313/boombrick/internal/library"
)

func TestFormatting(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{Clock(3*time.Minute + 42*time.Second), "3:42"},
		{Clock(72*time.Minute + 5*time.Second), "72:05"},
		{SongsAndTime(1, 0), "1 song"},
		{SongsAndTime(12, 46*time.Minute+20*time.Second), "12 songs · 46 min"},
		{SongsAndTime(40, 125*time.Minute), "40 songs · 2 h 5 min"},
		{Count(7), "7"},
		{Count(2431), "2,431"},
		{Count(1234567), "1,234,567"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestTrackOf(t *testing.T) {
	lt := library.Track{Path: "/m/x.flac", Title: "Rust", Artist: "Odalys", Album: "Copper Season",
		Gain: -6.5, Peak: 0.9, HasGain: true, Duration: time.Minute}
	pt := TrackOf(&lt)
	if pt.AlbumArtist != "Odalys" || pt.Path != lt.Path || pt.Gain != -6.5 || !pt.HasGain || pt.Duration != time.Minute {
		t.Fatalf("TrackOf = %+v", pt)
	}
	lt.AlbumArtist = "Various Artists"
	if TrackOf(&lt).AlbumArtist != "Various Artists" {
		t.Fatal("an album artist tag wins")
	}
}
