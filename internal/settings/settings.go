// Package settings stores user preferences in the settings table and falls
// back to the defaults documented in docs/design.md.
package settings

import (
	"database/sql"
	"strconv"
	"sync"
)

// Keys of every persisted setting.
const (
	StreamingKbps        = "spotify.streaming_kbps"
	SpotifyConnect       = "spotify.connect"
	SpotifyClientID      = "spotify.client_id"
	DownloadQuality      = "downloads.quality"
	PreferDownloaded     = "playback.prefer_downloaded"
	VolumeLevelling      = "playback.volume_levelling"
	EQPreset             = "playback.eq"
	CrossfadeSeconds     = "playback.crossfade_seconds"
	RemoveDeletedImports = "import.remove_deleted"
)

// Defaults holds the value used for a key that has never been set.
var Defaults = map[string]string{
	StreamingKbps:        "160",
	SpotifyConnect:       "true",
	SpotifyClientID:      "",
	DownloadQuality:      "best",
	PreferDownloaded:     "true",
	VolumeLevelling:      "true",
	EQPreset:             "flat",
	CrossfadeSeconds:     "0",
	RemoveDeletedImports: "false",
}

// Settings keeps the table in memory so reads never touch the database.
type Settings struct {
	db     *sql.DB
	mu     sync.RWMutex
	values map[string]string
}

// Load reads every stored setting.
func Load(db *sql.DB) (*Settings, error) {
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	s := &Settings{db: db, values: map[string]string{}}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		s.values[k] = v
	}
	return s, rows.Err()
}

// String returns the stored value or the default.
func (s *Settings) String(key string) string {
	s.mu.RLock()
	v, ok := s.values[key]
	s.mu.RUnlock()
	if !ok {
		return Defaults[key]
	}
	return v
}

// Int returns the setting as an integer, or the default if the stored value
// isn't a number.
func (s *Settings) Int(key string) int {
	if n, err := strconv.Atoi(s.String(key)); err == nil {
		return n
	}
	n, _ := strconv.Atoi(Defaults[key])
	return n
}

// Bool returns the setting as a boolean, or the default if the stored value
// isn't one.
func (s *Settings) Bool(key string) bool {
	if b, err := strconv.ParseBool(s.String(key)); err == nil {
		return b
	}
	b, _ := strconv.ParseBool(Defaults[key])
	return b
}

// Set stores a value.
func (s *Settings) Set(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.values[key] = value
	s.mu.Unlock()
	return nil
}

// SetInt stores an integer.
func (s *Settings) SetInt(key string, v int) error { return s.Set(key, strconv.Itoa(v)) }

// SetBool stores a boolean.
func (s *Settings) SetBool(key string, v bool) error { return s.Set(key, strconv.FormatBool(v)) }
