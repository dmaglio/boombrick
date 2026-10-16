package settings

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ujjwal313/boombrick/internal/store"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st.DB
}

func TestDefaults(t *testing.T) {
	s, err := Load(openDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Int(StreamingKbps); got != 160 {
		t.Errorf("StreamingKbps = %d, want 160", got)
	}
	if !s.Bool(PreferDownloaded) {
		t.Error("PreferDownloaded should default to true")
	}
	if got := s.String(EQPreset); got != "flat" {
		t.Errorf("EQPreset = %q, want flat", got)
	}
}

func TestSetPersists(t *testing.T) {
	db := openDB(t)
	s, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetInt(StreamingKbps, 320); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBool(PreferDownloaded, false); err != nil {
		t.Fatal(err)
	}

	again, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Int(StreamingKbps); got != 320 {
		t.Errorf("StreamingKbps = %d after reload, want 320", got)
	}
	if again.Bool(PreferDownloaded) {
		t.Error("PreferDownloaded should be false after reload")
	}
}

func TestInvalidValueFallsBackToDefault(t *testing.T) {
	s, err := Load(openDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(CrossfadeSeconds, "lots"); err != nil {
		t.Fatal(err)
	}
	if got := s.Int(CrossfadeSeconds); got != 0 {
		t.Errorf("CrossfadeSeconds = %d, want default 0", got)
	}
}
