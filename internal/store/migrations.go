package store

import (
	"database/sql"
	"fmt"
)

// migrations run in order; PRAGMA user_version records how many have been
// applied. Append new entries and never edit ones that have shipped.
var migrations = []string{
	`CREATE TABLE settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	) WITHOUT ROWID`,
	`CREATE TABLE tracks (
		id     INTEGER PRIMARY KEY,
		path   TEXT NOT NULL UNIQUE,
		size   INTEGER NOT NULL,
		mtime  INTEGER NOT NULL,
		artist TEXT NOT NULL,
		album  TEXT NOT NULL,
		title  TEXT NOT NULL,
		number INTEGER NOT NULL,
		added  INTEGER NOT NULL
	);
	CREATE INDEX tracks_album ON tracks (artist, album)`,
	`ALTER TABLE tracks ADD COLUMN album_artist TEXT NOT NULL DEFAULT '';
	ALTER TABLE tracks ADD COLUMN disc INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE tracks ADD COLUMN year INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE tracks ADD COLUMN gain REAL;
	ALTER TABLE tracks ADD COLUMN peak REAL;
	UPDATE tracks SET mtime = 0;
	CREATE TABLE queue (
		pos          INTEGER PRIMARY KEY,
		path         TEXT NOT NULL,
		title        TEXT NOT NULL,
		artist       TEXT NOT NULL,
		album_artist TEXT NOT NULL,
		album        TEXT NOT NULL,
		gain         REAL NOT NULL,
		peak         REAL NOT NULL,
		has_gain     INTEGER NOT NULL
	);
	CREATE TABLE playback_state (
		id         INTEGER PRIMARY KEY CHECK (id = 1),
		pos        INTEGER NOT NULL,
		frame      INTEGER NOT NULL,
		shuffle    INTEGER NOT NULL,
		repeat     INTEGER NOT NULL,
		play_order TEXT NOT NULL
	);
	CREATE TABLE progress (
		artist   TEXT NOT NULL,
		album    TEXT NOT NULL,
		path     TEXT NOT NULL,
		frame    INTEGER NOT NULL,
		fraction REAL NOT NULL,
		updated  INTEGER NOT NULL,
		PRIMARY KEY (artist, album)
	)`,
	`ALTER TABLE tracks ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE tracks ADD COLUMN album_gain REAL;
	ALTER TABLE tracks ADD COLUMN album_peak REAL;
	UPDATE tracks SET mtime = 0;
	ALTER TABLE queue ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE queue ADD COLUMN album_gain REAL NOT NULL DEFAULT 0;
	ALTER TABLE queue ADD COLUMN album_peak REAL NOT NULL DEFAULT 0;
	ALTER TABLE queue ADD COLUMN has_album_gain INTEGER NOT NULL DEFAULT 0;
	UPDATE progress SET updated = updated * 1000`,
	`CREATE TABLE art (
		key          TEXT PRIMARY KEY,
		file         TEXT NOT NULL,
		source       TEXT NOT NULL,
		track        TEXT NOT NULL,
		track_mtime  INTEGER NOT NULL,
		dir_mtime    INTEGER NOT NULL,
		source_mtime INTEGER NOT NULL,
		hue          REAL NOT NULL,
		chroma       REAL NOT NULL,
		colour       INTEGER NOT NULL,
		looked_up    INTEGER NOT NULL
	)`,
	`CREATE TABLE lyrics (
		path         TEXT PRIMARY KEY,
		body         TEXT NOT NULL,
		instrumental INTEGER NOT NULL,
		fetched      INTEGER NOT NULL
	)`,
}

func migrate(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&applied); err != nil {
		return err
	}
	for i := applied; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
