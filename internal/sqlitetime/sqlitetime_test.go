package sqlitetime_test

import (
	"context"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"

	"github.com/lesomnus/cxz/internal/sqlitetime"
)

// A database written before the driver stored times as integers: the text the
// old default wrote, with its fraction trimmed, beside a value already in the
// new format and a NULL. After Migrate every one of them reads back as the time
// it was, and running it again changes nothing.
func TestMigrateTextTimes(t *testing.T) {
	ctx := context.Background()
	dsn := (&url.URL{Scheme: "file", Path: filepath.Join(t.TempDir(), "resources.db")}).String()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: dsn, MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE project(id TEXT PRIMARY KEY, name TEXT, date_updated datetime NOT NULL, date_erased datetime NULL)"); err != nil {
		t.Fatal(err)
	}
	want := map[string]time.Time{
		"nanos":   time.Date(2026, 9, 19, 23, 53, 7, 646468039, time.UTC),
		"trimmed": time.Date(2026, 9, 19, 23, 53, 7, 93000000, time.UTC),
		"whole":   time.Date(2026, 9, 19, 23, 53, 7, 0, time.UTC),
		// Not UTC, so not the statement's: the parse takes it.
		"offset":  time.Date(2026, 9, 20, 8, 53, 7, 120000000, time.FixedZone("KST", 9*60*60)),
		"integer": time.Date(2026, 10, 30, 1, 2, 3, 4, time.UTC),
	}
	for id, at := range want {
		if id == "integer" {
			// Written through the driver, which is the new format.
			_, err = db.Exec("INSERT INTO project VALUES(?,?,?,NULL)", id, "x", at)
		} else {
			// Written as the old default did: text, trailing zeros trimmed.
			_, err = db.Exec("INSERT INTO project VALUES(?,?,?,NULL)", id, "2026", at.Format(time.RFC3339Nano))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	var got time.Time
	if err = db.QueryRow("SELECT date_updated FROM project WHERE id='nanos'").Scan(&got); err == nil {
		t.Fatal("the fixture's text is readable as a time, so it does not reproduce the failure")
	}
	for range 2 {
		if err = sqlitetime.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
		for id, at := range want {
			var erased *time.Time
			if err = db.QueryRow("SELECT date_updated, date_erased FROM project WHERE id=?", id).Scan(&got, &erased); err != nil {
				t.Fatal(id, err)
			}
			if !got.Equal(at) || erased != nil {
				t.Fatal(id, got, at, erased)
			}
		}
	}
	// The text column with a date-looking value is not a time column.
	var name string
	if err = db.QueryRow("SELECT name FROM project WHERE id='nanos'").Scan(&name); err != nil || name != "2026" {
		t.Fatal(name, err)
	}
}

// A time column holding text that is not a time is reported, not skipped: the
// read it would fail is no better for being put off.
func TestMigrateRefusesWhatIsNotATime(t *testing.T) {
	ctx := context.Background()
	dsn := (&url.URL{Scheme: "file", Path: filepath.Join(t.TempDir(), "resources.db")}).String()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: dsn, MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE audit(id TEXT PRIMARY KEY, date_created datetime NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO audit VALUES('a','yesterday')"); err != nil {
		t.Fatal(err)
	}
	if err = sqlitetime.Migrate(ctx, db); err == nil {
		t.Fatal("migrated a value that is not a time")
	}
}
