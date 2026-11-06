package middleware

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

// icsFakeDB stubs the queries CalendarICSHandler makes and counts its writes.
type icsFakeDB struct {
	dbtypes.Querier

	found          bool
	accessUpdates  int
	missingRecords int
}

func (f *icsFakeDB) GetURLMapping(context.Context, string) (dbtypes.UrlMapping, error) {
	if !f.found {
		return dbtypes.UrlMapping{}, pgx.ErrNoRows
	}
	return dbtypes.UrlMapping{ValueList: []byte(`{}`)}, nil
}

func (f *icsFakeDB) UpdateURLMappingAccessCount(context.Context, string) error {
	f.accessUpdates++
	return nil
}

func (f *icsFakeDB) RecordMissingCalendar(
	context.Context, dbtypes.RecordMissingCalendarParams,
) (pgtype.Timestamp, error) {
	f.missingRecords++
	return pgtype.Timestamp{}, nil
}

// icsFakeCache is an always-empty cache that counts ICS deletions.
type icsFakeCache struct {
	Cache

	deletes int
}

func (*icsFakeCache) GetICS(string) (string, bool) { return "", false }

func (*icsFakeCache) SetICS(string, string) error { return nil }

func (f *icsFakeCache) DeleteICS(string) error {
	f.deletes++
	return nil
}

func TestCalendarICSHandler_HeadIgnoresRefresh(t *testing.T) {
	t.Parallel()
	for method, wantDeletes := range map[string]int{http.MethodGet: 1, http.MethodHead: 0} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			cache := &icsFakeCache{}
			m := &Middleware{
				Logger:     zap.NewNop(),
				Context:    context.Background(),
				DBConn:     &icsFakeDB{found: true},
				RedisCache: cache,
			}
			c, _ := newTestContext(t, method, "/"+strings.Repeat("a", calendarHashLen)+".ics?refresh=1", nil)

			m.CalendarICSHandler(c)

			if cache.deletes != wantDeletes {
				t.Fatalf("cache deletes = %d, want %d", cache.deletes, wantDeletes)
			}
		})
	}
}

func TestCalendarICSHandler_HeadRecordsNothing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		found       bool
		method      string
		wantAccess  int
		wantMissing int
	}{
		{name: "GET known calendar", found: true, method: http.MethodGet, wantAccess: 1},
		{name: "HEAD known calendar", found: true, method: http.MethodHead},
		{name: "GET expired calendar", method: http.MethodGet, wantMissing: 1},
		{name: "HEAD expired calendar", method: http.MethodHead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := &icsFakeDB{found: tt.found}
			m := &Middleware{Logger: zap.NewNop(), Context: context.Background(), DBConn: db}
			c, w := newTestContext(t, tt.method, "/"+strings.Repeat("a", calendarHashLen)+".ics", nil)

			m.CalendarICSHandler(c)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if got := w.Header().Get("Content-Type"); got != "text/calendar; charset=utf-8" {
				t.Fatalf("Content-Type = %q", got)
			}
			if db.accessUpdates != tt.wantAccess {
				t.Fatalf("access updates = %d, want %d", db.accessUpdates, tt.wantAccess)
			}
			if db.missingRecords != tt.wantMissing {
				t.Fatalf("missing records = %d, want %d", db.missingRecords, tt.wantMissing)
			}
		})
	}
}
