package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"
)

type uniqueEvent struct {
	kind   string // "visitor" or "calendar"
	day    string
	member string
}

// fakeUniques is an in-memory UniqueStore. Recording happens in a goroutine,
// so adds are delivered on a channel for tests to wait on.
type fakeUniques struct {
	events    chan uniqueEvent
	countDays map[int]int64 // window length in days -> count returned
	countErr  error
}

func newFakeUniques() *fakeUniques {
	return &fakeUniques{events: make(chan uniqueEvent, 16), countDays: map[int]int64{}}
}

func (f *fakeUniques) AddVisitor(_ context.Context, day, visitorID string) error {
	f.events <- uniqueEvent{kind: "visitor", day: day, member: visitorID}
	return nil
}

func (f *fakeUniques) AddCalendar(_ context.Context, day, hash string) error {
	f.events <- uniqueEvent{kind: "calendar", day: day, member: hash}
	return nil
}

func (f *fakeUniques) CountVisitors(_ context.Context, days []string) (int64, error) {
	return f.countDays[len(days)], f.countErr
}

func (f *fakeUniques) CountCalendars(_ context.Context, days []string) (int64, error) {
	return f.countDays[len(days)] * 10, f.countErr
}

func (f *fakeUniques) next(t *testing.T) uniqueEvent {
	t.Helper()
	select {
	case ev := <-f.events:
		return ev
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unique event")
		return uniqueEvent{}
	}
}

func newTestTelemetry(t *testing.T, uniques UniqueStore) (*Telemetry, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel, err := NewTelemetry(provider.Meter("test"), uniques, nil, zaptest.NewLogger(t))
	if err != nil {
		t.Fatalf("NewTelemetry: %v", err)
	}
	return tel, reader
}

func newTelemetryRouter(tel *Telemetry) *gin.Engine {
	r := gin.New()
	r.Use(tel.Middleware())
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.GET("/", ok)
	r.GET("/static/*filepath", ok)
	r.GET("/robots.txt", ok)
	r.NoRoute(func(c *gin.Context) {
		if c.Request.URL.Path == "/known.ics" {
			c.Status(http.StatusOK)
			return
		}
		c.Status(http.StatusNotFound)
	})
	return r
}

func serve(r *gin.Engine, path, ip, ua string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = ip + ":1234"
	req.Header.Set("User-Agent", ua)
	r.ServeHTTP(httptest.NewRecorder(), req)
}

// collect returns the named metric from the reader, or fails the test.
func collect(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Metrics{}
}

func attr(set attribute.Set, key attribute.Key) string {
	v, _ := set.Value(key)
	return v.String()
}

func TestTelemetryMiddleware_CountsRequestsByRoute(t *testing.T) {
	t.Parallel()
	tel, reader := newTestTelemetry(t, nil)
	r := newTelemetryRouter(tel)

	for _, path := range []string{"/", "/", "/known.ics", "/missing.ics", "/wp-login.php", "/static/css/app.css", "/robots.txt"} {
		serve(r, path, "1.2.3.4", "ua")
	}

	sum, ok := collect(t, reader, "esportscalendar.http.requests").Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatal("request metric is not an int64 sum")
	}
	got := map[string]int64{}
	for _, dp := range sum.DataPoints {
		got[attr(dp.Attributes, "http.route")+" "+attr(dp.Attributes, "http.response.status_code")] = dp.Value
		if m := attr(dp.Attributes, "http.request.method"); m != http.MethodGet {
			t.Errorf("method = %q, want GET", m)
		}
	}
	want := map[string]int64{
		"/ 200":          2,
		"/:hash.ics 200": 1,
		"/:hash.ics 404": 1,
		"unmatched 404":  1,
	}
	if len(got) != len(want) {
		t.Fatalf("data points = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
}

func TestTelemetryMiddleware_RecordsUniques(t *testing.T) {
	t.Parallel()
	uniques := newFakeUniques()
	tel, _ := newTestTelemetry(t, uniques)
	r := newTelemetryRouter(tel)

	serve(r, "/", "1.2.3.4", "ua-a")
	first := uniques.next(t)
	serve(r, "/", "1.2.3.4", "ua-a")
	same := uniques.next(t)
	serve(r, "/", "1.2.3.4", "ua-b")
	other := uniques.next(t)

	if first.kind != "visitor" {
		t.Fatalf("kind = %q, want visitor", first.kind)
	}
	if first.day != time.Now().UTC().Format(uniqueDayFormat) {
		t.Fatalf("day = %q, want today (UTC)", first.day)
	}
	if first.member == "" || first.member == "1.2.3.4" {
		t.Fatalf("visitor id = %q, want an opaque hash", first.member)
	}
	if same.member != first.member {
		t.Fatalf("same client got different ids: %q vs %q", first.member, same.member)
	}
	if other.member == first.member {
		t.Fatalf("different user agents got the same id %q", first.member)
	}
}

func TestTelemetryMiddleware_SkipsNonVisitorUniques(t *testing.T) {
	t.Parallel()
	uniques := newFakeUniques()
	tel, _ := newTestTelemetry(t, uniques)
	r := newTelemetryRouter(tel)

	for _, path := range []string{"/static/css/app.css", "/robots.txt", "/wp-login.php", "/missing.ics"} {
		serve(r, path, "1.2.3.4", "ua")
	}
	// Recording is async, so send a recorded request last: if it is the first
	// event to arrive, none of the paths above were recorded.
	serve(r, "/known.ics", "1.2.3.4", "ua")

	ev := uniques.next(t)
	if ev.kind != "calendar" || ev.member != "known" {
		t.Fatalf("first event = %+v, want calendar %q", ev, "known")
	}
	select {
	case extra := <-uniques.events:
		t.Fatalf("unexpected extra event %+v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTelemetry_UniqueGauges(t *testing.T) {
	t.Parallel()
	uniques := newFakeUniques()
	uniques.countDays = map[int]int64{1: 3, 7: 12, 30: 40}
	_, reader := newTestTelemetry(t, uniques)

	for name, scale := range map[string]int64{
		"esportscalendar.visitors.unique":  1,
		"esportscalendar.calendars.unique": 10,
	} {
		gauge, ok := collect(t, reader, name).Data.(metricdata.Gauge[int64])
		if !ok {
			t.Fatalf("%s is not an int64 gauge", name)
		}
		got := map[string]int64{}
		for _, dp := range gauge.DataPoints {
			got[attr(dp.Attributes, "window")] = dp.Value
		}
		want := map[string]int64{"today": 3 * scale, "7d": 12 * scale, "30d": 40 * scale}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s window=%s = %d, want %d", name, k, got[k], v)
			}
		}
	}
}

func TestTelemetry_UniqueGaugesSkipOnError(t *testing.T) {
	t.Parallel()
	uniques := newFakeUniques()
	uniques.countErr = errors.New("redis down")
	_, reader := newTestTelemetry(t, uniques)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok && len(g.DataPoints) > 0 {
				t.Fatalf("%s reported %d points despite store error", m.Name, len(g.DataPoints))
			}
		}
	}
}

func TestTelemetry_NoGaugesWithoutStore(t *testing.T) {
	t.Parallel()
	tel, reader := newTestTelemetry(t, nil)
	serve(newTelemetryRouter(tel), "/", "1.2.3.4", "ua") // must not panic

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "esportscalendar.http.requests" {
				t.Fatalf("unexpected metric %q without a store", m.Name)
			}
		}
	}
}

func TestVisitorID_PrefersCloudflareHeader(t *testing.T) {
	t.Parallel()
	c, _ := newTestContext(t, http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "10.0.0.1:1234"
	withoutCF := visitorID(c)

	c.Request.Header.Set("CF-Connecting-IP", "9.9.9.9")
	withCF := visitorID(c)

	if withCF == withoutCF {
		t.Fatal("CF-Connecting-IP was ignored")
	}
}

func TestLastDays(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60))
	got := lastDays(now, 3)
	want := []string{"2026-03-02", "2026-03-01", "2026-02-28"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("lastDays = %v, want %v", got, want)
		}
	}
}
