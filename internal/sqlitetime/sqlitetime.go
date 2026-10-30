// Package sqlitetime moves a SQLite database's times to the format the driver
// now reads them in.
//
// payday opens every sqlite3 database with `_timefmt=unixepoch_nano`, because
// the driver's default -- RFC3339 text with the fraction's trailing zeros
// trimmed -- does not sort as text: `05:38:42Z` is greater than
// `05:38:42.093Z`. With it the driver writes a time as integer nanoseconds and
// reads one back from an integer. A time it finds written as text it hands back
// as a string, which no `time.Time` accepts, so a database written before the
// change failed every read of a row it already had:
//
//	sql: Scan error on column index 6, name "date_updated": unsupported Scan,
//	storing driver.Value type string into type *time.Time
//
// [Migrate] rewrites those values as the integers the driver would have
// written. It touches only text in columns declared as times, so it is a no-op
// on a database that has nothing left to move.
package sqlitetime

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// batch bounds how long one transaction holds the write lock: the audit table
// of a real installation had 731,246 rows to move.
const batch = 5000

// Migrate rewrites every time stored as text in a column declared DATETIME,
// TIMESTAMP or DATE as integer nanoseconds since the epoch. A value that does
// not parse as RFC3339 is an error rather than skipped: it would fail the read
// anyway, and saying which one is the part that helps.
func Migrate(ctx context.Context, db *sql.DB) error {
	columns, err := timeColumns(ctx, db)
	if err != nil {
		return err
	}
	for _, c := range columns {
		if err := migrateColumn(ctx, db, c); err != nil {
			return fmt.Errorf("migrate times in %s.%s: %w", c.table, c.name, err)
		}
	}
	return nil
}

type column struct{ table, name string }

func timeColumns(ctx context.Context, db *sql.DB) ([]column, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var t string
		if err = rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, t)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var out []column
	for _, t := range tables {
		rows, err := db.QueryContext(ctx, `SELECT name, type FROM pragma_table_info(?)`, t)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var name, typ string
			if err = rows.Scan(&name, &typ); err != nil {
				rows.Close()
				return nil, err
			}
			switch strings.ToLower(typ) {
			case "datetime", "timestamp", "date":
				out = append(out, column{t, name})
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func migrateColumn(ctx context.Context, db *sql.DB, c column) error {
	table, name := quote(c.table), quote(c.name)
	// What the old default wrote is UTC with a `Z` and up to nine digits of
	// fraction, and SQLite can turn that into nanoseconds itself: a statement
	// per column instead of a round trip per row, which is the difference
	// between a second and a minute and a half on that audit table. Whatever
	// it does not match is left for the parse below.
	if _, err := db.ExecContext(ctx, `UPDATE `+table+` SET `+name+` = `+
		`CAST(strftime('%s', substr(`+name+`, 1, 19)) AS INTEGER) * 1000000000 + `+
		`CASE WHEN length(`+name+`) = 20 THEN 0 ELSE CAST(substr(substr(`+name+`, 21, length(`+name+`) - 21) || '000000000', 1, 9) AS INTEGER) END `+
		`WHERE typeof(`+name+`) = 'text' `+
		`AND `+name+` GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]*Z' `+
		`AND strftime('%s', substr(`+name+`, 1, 19)) IS NOT NULL `+
		`AND (length(`+name+`) = 20 OR (substr(`+name+`, 20, 1) = '.' AND length(`+name+`) BETWEEN 22 AND 30 AND substr(`+name+`, 21, length(`+name+`) - 21) NOT GLOB '*[^0-9]*'))`); err != nil {
		return err
	}
	for {
		n, err := migrateBatch(ctx, db, table, name)
		if err != nil {
			return err
		}
		if n < batch {
			return nil
		}
	}
}

// migrateBatch moves up to one batch, by rowid: every table here is a rowid
// table, and a WITHOUT ROWID one is refused by SQLite rather than skipped.
func migrateBatch(ctx context.Context, db *sql.DB, table, name string) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT rowid, `+name+` FROM `+table+` WHERE typeof(`+name+`)='text' LIMIT ?`, batch)
	if err != nil {
		return 0, err
	}
	type value struct {
		rowid int64
		at    int64
	}
	var values []value
	for rows.Next() {
		var rowid int64
		var text string
		if err = rows.Scan(&rowid, &text); err != nil {
			rows.Close()
			return 0, err
		}
		at, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			rows.Close()
			return 0, fmt.Errorf("row %d: %w", rowid, err)
		}
		values = append(values, value{rowid, at.UnixNano()})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	update, err := tx.PrepareContext(ctx, `UPDATE `+table+` SET `+name+`=? WHERE rowid=?`)
	if err != nil {
		return 0, err
	}
	defer update.Close()
	for _, v := range values {
		if _, err = update.ExecContext(ctx, v.at, v.rowid); err != nil {
			return 0, err
		}
	}
	return len(values), tx.Commit()
}
