//go:build dbtest

package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/feimaomiao/esportscalendar/dbtypes"
	"github.com/feimaomiao/esportscalendar/internal/testdb"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) { testdb.RunMain(m, &pool) }

func resetMissingCalendars(t *testing.T) *dbtypes.Queries {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE url_mappings, missing_calendars`); err != nil {
		t.Fatalf("reset: %v", err)
	}
	return dbtypes.New(pool)
}

func TestRecordMissingCalendarIsCapped(t *testing.T) {
	ctx := context.Background()
	db := resetMissingCalendars(t)
	record := func(hash string) error {
		_, err := db.RecordMissingCalendar(ctx, dbtypes.RecordMissingCalendarParams{HashedKey: hash, MaxRows: 2})
		return err
	}
	for _, hash := range []string{"aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"} {
		if err := record(hash); err != nil {
			t.Fatalf("record %s: %v", hash, err)
		}
	}

	if err := record("cccccccccccccccc"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("over the cap: err = %v, want no rows", err)
	}
	// A real lost link already on the list keeps its count once the table fills.
	if err := record("aaaaaaaaaaaaaaaa"); err != nil {
		t.Fatalf("known hash at the cap: %v", err)
	}

	var rows, hits int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*), MAX(hit_count) FROM missing_calendars`).Scan(&rows, &hits)
	if err != nil {
		t.Fatalf("read table: %v", err)
	}
	if rows != 2 || hits != 2 {
		t.Fatalf("rows = %d, max hit_count = %d, want 2 and 2", rows, hits)
	}
}

func TestUnrecoveredFollowsURLMappings(t *testing.T) {
	ctx := context.Background()
	db := resetMissingCalendars(t)
	const stillLost, reExported, lostAgain = "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb", "cccccccccccccccc"
	for _, stmt := range []string{
		`INSERT INTO missing_calendars (hashed_key) VALUES ('` + stillLost + `'), ('` + reExported + `')`,
		// Marked recovered, but its mapping is gone: lost a second time.
		`INSERT INTO missing_calendars (hashed_key, recovered_at) VALUES ('` + lostAgain + `', NOW())`,
		// Restored by a user re-exporting, so recovered_at was never set.
		`INSERT INTO url_mappings (hashed_key, value_list, access_count) VALUES ('` + reExported + `', '{}', 0)`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got, err := db.GetUnrecoveredMissingCalendars(ctx)
	if err != nil {
		t.Fatalf("GetUnrecoveredMissingCalendars: %v", err)
	}
	if len(got) != 2 || got[0] != stillLost || got[1] != lostAgain {
		t.Fatalf("got %v, want [%s %s]", got, stillLost, lostAgain)
	}
}
