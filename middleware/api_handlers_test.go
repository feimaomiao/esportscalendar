package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v4"
	"go.uber.org/zap/zaptest"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

// fakeCache is an in-memory implementation of Cache for tests. It satisfies
// the same surface as *RedisCache without touching Redis.
type fakeCache struct {
	bytes map[string][]byte
	data  map[string]string
	ics   map[string]string

	setBytesErr error
	setDataErr  error
	setICSErr   error
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		bytes: make(map[string][]byte),
		data:  make(map[string]string),
		ics:   make(map[string]string),
	}
}

func (f *fakeCache) GetBytes(k string) ([]byte, bool) {
	v, ok := f.bytes[k]
	return v, ok
}

func (f *fakeCache) SetBytes(k string, v []byte) error {
	if f.setBytesErr != nil {
		return f.setBytesErr
	}
	f.bytes[k] = append([]byte(nil), v...)
	return nil
}

func (f *fakeCache) GetData(k string) (string, bool) {
	v, ok := f.data[k]
	return v, ok
}

func (f *fakeCache) SetData(k string, v string) error {
	if f.setDataErr != nil {
		return f.setDataErr
	}
	f.data[k] = v
	return nil
}

func (f *fakeCache) GetICS(h string) (string, bool) {
	v, ok := f.ics[h]
	return v, ok
}

func (f *fakeCache) SetICS(h string, c string) error {
	if f.setICSErr != nil {
		return f.setICSErr
	}
	f.ics[h] = c
	return nil
}

func (f *fakeCache) DeleteICS(h string) error {
	delete(f.ics, h)
	return nil
}

func (f *fakeCache) Close() error { return nil }

// Compile-time check that *fakeCache satisfies Cache.
var _ Cache = (*fakeCache)(nil)

// newLeagueHandlerTest builds a Middleware backed by a pgxmock pool and an
// in-memory fakeCache, plus a Gin context wired for /api/league-options.
func newLeagueHandlerTest(
	t *testing.T,
	paramValue string,
) (*Middleware, pgxmock.PgxPoolIface, *fakeCache, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	mockDB, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mockDB.Close() })

	cache := newFakeCache()
	mw := &Middleware{
		DBConn:     dbtypes.New(mockDB),
		Context:    context.Background(),
		RedisCache: cache,
		Logger:     zaptest.NewLogger(t),
		BaseURL:    "https://test.example",
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = []gin.Param{{Key: "param", Value: paramValue}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/league-options"+paramValue, nil)
	return mw, mockDB, cache, c, w
}

func TestLeagueOptionsHandler_BadGameID(t *testing.T) {
	mw, mockDB, cache, c, w := newLeagueHandlerTest(t, "/abc")

	mw.LeagueOptionsHandler(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v (%q)", err, w.Body.String())
	}
	if body["error"] != true {
		t.Errorf(`body["error"] = %v, want true`, body["error"])
	}
	if body["message"] != "Invalid game ID" {
		t.Errorf(`body["message"] = %v, want "Invalid game ID"`, body["message"])
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("DB should not have been called: %v", err)
	}
	if len(cache.bytes) != 0 {
		t.Errorf("cache should be untouched, got %d entries", len(cache.bytes))
	}
}

func TestLeagueOptionsHandler_CacheHit(t *testing.T) {
	mw, mockDB, cache, c, w := newLeagueHandlerTest(t, "/1")
	cached := []byte(`{"error":false,"message":"","leagues":[{"id":99,"name":"From Cache"}]}`)
	cache.bytes["league-options:1"] = cached

	mw.LeagueOptionsHandler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("X-Cache"); got != "HIT" {
		t.Errorf("X-Cache = %q, want HIT", got)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := w.Body.String(); got != string(cached) {
		t.Errorf("body = %q, want %q", got, string(cached))
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("DB should not have been called: %v", err)
	}
}

func TestLeagueOptionsHandler_CacheMissDBHit(t *testing.T) {
	mw, mockDB, cache, c, w := newLeagueHandlerTest(t, "/1")

	// MinTier is interface{} in the sqlc-generated row; pgx scans an INT4 as
	// int32 in production, so the fixture must use int32(...) — using a plain
	// Go int silently fails the handler's tier.(int32) assertion and produces
	// is_tier1=false even for tier-1 rows.
	rows := pgxmock.NewRows([]string{"id", "name", "slug", "game_id", "image_link", "min_tier"}).
		AddRow(
			int32(10),
			"Top League",
			pgtype.Text{String: "top", Valid: true},
			int32(1),
			pgtype.Text{String: "https://cdn.example/top.png", Valid: true},
			int32(1),
		).
		AddRow(
			int32(20),
			"Other League",
			pgtype.Text{String: "other", Valid: true},
			int32(1),
			pgtype.Text{Valid: false},
			int32(3),
		)
	mockDB.ExpectQuery(`SELECT.+FROM leagues`).
		WithArgs(int32(1)).
		WillReturnRows(rows)

	mw.LeagueOptionsHandler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Cache"); got != "MISS" {
		t.Errorf("X-Cache = %q, want MISS", got)
	}

	var body struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
		Leagues []struct {
			ID      int32  `json:"id"`
			Name    string `json:"name"`
			Image   string `json:"image"`
			IsTier1 bool   `json:"is_tier1"`
		} `json:"leagues"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v (%q)", err, w.Body.String())
	}
	if body.Error {
		t.Errorf("body.error = true, want false")
	}
	if len(body.Leagues) != 2 {
		t.Fatalf("got %d leagues, want 2", len(body.Leagues))
	}
	if body.Leagues[0].ID != 10 || !body.Leagues[0].IsTier1 {
		t.Errorf("league[0] = %+v, want id=10 is_tier1=true", body.Leagues[0])
	}
	if body.Leagues[0].Image != "https://cdn.example/top.png" {
		t.Errorf("league[0].image = %q, want CDN URL", body.Leagues[0].Image)
	}
	if body.Leagues[1].ID != 20 || body.Leagues[1].IsTier1 {
		t.Errorf("league[1] = %+v, want id=20 is_tier1=false", body.Leagues[1])
	}
	if body.Leagues[1].Image != "/static/images/default-logo.png" {
		t.Errorf("league[1].image = %q, want default-logo fallback", body.Leagues[1].Image)
	}
	if _, ok := cache.bytes["league-options:1"]; !ok {
		t.Errorf("expected cache write for key league-options:1, got none")
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet DB expectations: %v", err)
	}
}

func TestLeagueOptionsHandler_DBError(t *testing.T) {
	mw, mockDB, cache, c, w := newLeagueHandlerTest(t, "/1")
	mockDB.ExpectQuery(`SELECT.+FROM leagues`).
		WithArgs(int32(1)).
		WillReturnError(errors.New("connection refused"))

	mw.LeagueOptionsHandler(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v (%q)", err, w.Body.String())
	}
	if body["error"] != true {
		t.Errorf(`body["error"] = %v, want true`, body["error"])
	}
	msg, _ := body["message"].(string)
	if !strings.Contains(msg, "Database connection error") {
		t.Errorf("message = %q, want it to mention 'Database connection error'", msg)
	}
	if len(cache.bytes) != 0 {
		t.Errorf("cache should not be written on DB error, got %d entries", len(cache.bytes))
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet DB expectations: %v", err)
	}
}
