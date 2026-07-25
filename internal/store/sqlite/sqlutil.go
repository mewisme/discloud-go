package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlitedrv "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mewisme/discloud-go/internal/store"
)

// tsLayout is RFC3339Nano with the fraction padded to a fixed nine digits.
// time.RFC3339Nano trims trailing zeros, which would break the lexicographic
// TEXT comparisons the schema relies on for expires_at / created_at ranges.
const tsLayout = "2006-01-02T15:04:05.000000000Z07:00"

// tsArg encodes a timestamp for a NOT NULL TEXT column.
func tsArg(t time.Time) string { return t.UTC().Format(tsLayout) }

// tsPtrArg encodes an optional timestamp; nil becomes SQL NULL.
func tsPtrArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return tsArg(*t)
}

// intoTS scans a TEXT timestamp column into dst; NULL yields the zero time.
func intoTS(dst *time.Time) sql.Scanner { return &tsScanner{dst: dst} }

// intoTSPtr scans a nullable TEXT timestamp column; NULL yields nil.
func intoTSPtr(dst **time.Time) sql.Scanner { return &tsScanner{ptr: dst} }

type tsScanner struct {
	dst *time.Time
	ptr **time.Time
}

func (s *tsScanner) Scan(src any) error {
	var t time.Time
	switch v := src.(type) {
	case nil:
		if s.ptr != nil {
			*s.ptr = nil
			return nil
		}
		*s.dst = time.Time{}
		return nil
	case time.Time:
		t = v.UTC()
	case string:
		parsed, err := parseTS(v)
		if err != nil {
			return err
		}
		t = parsed
	case []byte:
		parsed, err := parseTS(string(v))
		if err != nil {
			return err
		}
		t = parsed
	default:
		return fmt.Errorf("sqlite: cannot scan %T into time.Time", src)
	}
	if s.ptr != nil {
		*s.ptr = &t
		return nil
	}
	*s.dst = t
	return nil
}

func parseTS(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("sqlite: parse timestamp %q: %w", s, err)
	}
	return t.UTC(), nil
}

// nullText returns nil for the empty string so optional TEXT columns stay NULL.
func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// deref returns the value behind an optional TEXT column, or "".
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// botArg maps the sentinel bot slot -1 to SQL NULL.
func botArg(botID int) any {
	if botID < 0 {
		return nil
	}
	return botID
}

// botValue maps a scanned NULL bot slot back to the sentinel -1.
func botValue(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

// placeholders renders "?, ?, ..." for an IN (...) list of n values.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// mapErr is the single translation point from driver errors to store errors:
// missing rows become ErrNotFound and uniqueness violations become ErrConflict.
// The remaining SQLITE_CONSTRAINT_* codes (CHECK, FOREIGN KEY, NOT NULL) are
// programming errors, and SQLITE_BUSY / SQLITE_LOCKED are transient contention
// that busy_timeout already retries; both pass through unchanged so callers see
// the real cause instead of a misleading conflict.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	var serr *sqlitedrv.Error
	if errors.As(err, &serr) {
		switch serr.Code() {
		case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
			return store.ErrConflict
		case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED:
			return err
		}
		if serr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT {
			return err
		}
	}
	return err
}

// affected reports ErrNotFound when a conditional UPDATE/DELETE matched nothing.
func affected(res sql.Result, err error) error {
	if err != nil {
		return mapErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
