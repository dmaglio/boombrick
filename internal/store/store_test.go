package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrationsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boombrick.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	var version int
	if err := s.DB.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Fatalf("user_version = %d, want %d", version, len(migrations))
	}

	var mode string
	if err := s.DB.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}

	if _, err := s.DB.Exec(`INSERT INTO settings (key, value) VALUES ('k', 'v')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening must not re-run migrations or lose data.
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var v string
	if err := s.DB.QueryRow(`SELECT value FROM settings WHERE key = 'k'`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "v" {
		t.Fatalf("value = %q", v)
	}
}

func TestMigrationThreeKeepsTracks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boombrick.db")
	old, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{migrations[0], migrations[1], `PRAGMA user_version = 2`,
		`INSERT INTO tracks (path, size, mtime, artist, album, title, number, added)
			VALUES ('/m/a.mp3', 10, 1700000000, 'Odalys', 'Copper Season', 'Rust', 1003, 5)`} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var artist, albumArtist string
	var size, mtime, disc, number, year int
	var gain, peak sql.NullFloat64
	err = s.DB.QueryRow(`SELECT artist, album_artist, size, mtime, disc, number, year, gain, peak FROM tracks`).
		Scan(&artist, &albumArtist, &size, &mtime, &disc, &number, &year, &gain, &peak)
	if err != nil {
		t.Fatal(err)
	}
	if artist != "Odalys" || albumArtist != "" || size != 10 || number != 1003 || disc != 0 || year != 0 ||
		gain.Valid || peak.Valid {
		t.Fatalf("row = %q %q size %d disc %d number %d year %d gain %v peak %v",
			artist, albumArtist, size, disc, number, year, gain, peak)
	}
	if mtime != 0 {
		t.Fatalf("mtime = %d, want 0 so the next scan re-reads the file", mtime)
	}
	for _, table := range []string{"queue", "playback_state", "progress"} {
		if _, err := s.DB.Exec(`SELECT 1 FROM ` + table); err != nil {
			t.Errorf("%s: %v", table, err)
		}
	}
}
