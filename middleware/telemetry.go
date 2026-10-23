package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.uber.org/zap"
)

const (
	// HyperLogLog key prefixes for unique counts. Kept separate from the cache
	// prefixes so RedisCache.Clear() never wipes them.
	uniqueVisitorsPrefix  = "stats:uv:"  // hashed visitor IDs per day
	uniqueCalendarsPrefix = "stats:ics:" // calendar hashes fetched per day

	uniqueKeyTTL         = 90 * 24 * time.Hour
	uniqueRecordTimeout  = 2 * time.Second
	uniqueDayFormat      = "2006-01-02"
	visitorIDBytes       = 16
	calendarRoute        = "/:hash.ics"
	unmatchedRoute       = "unmatched"
	telemetryMeterPrefix = "esportscalendar."
)

// uniqueWindows are the rolling windows (in UTC days, including today) the
// unique-count gauges report.
//
//nolint:gochecknoglobals // fixed lookup table
var uniqueWindows = []struct {
	name string
	days int
}{
	{"today", 1},
	{"7d", 7},
	{"30d", 30},
}

// UniqueStore tracks unique visitors and calendars per UTC day. Implemented in
// production by *RedisCache so uniques are de-duplicated across all replicas.
type UniqueStore interface {
	AddVisitor(ctx context.Context, day, visitorID string) error
	AddCalendar(ctx context.Context, day, hash string) error
	CountVisitors(ctx context.Context, days []string) (int64, error)
	CountCalendars(ctx context.Context, days []string) (int64, error)
}

// AddVisitor adds a visitor to the day's HyperLogLog.
func (c *RedisCache) AddVisitor(ctx context.Context, day, visitorID string) error {
	return c.addUnique(ctx, uniqueVisitorsPrefix+day, visitorID)
}

// AddCalendar adds a calendar hash to the day's HyperLogLog.
func (c *RedisCache) AddCalendar(ctx context.Context, day, hash string) error {
	return c.addUnique(ctx, uniqueCalendarsPrefix+day, hash)
}

// CountVisitors returns the number of unique visitors across the given days.
func (c *RedisCache) CountVisitors(ctx context.Context, days []string) (int64, error) {
	return c.client.PFCount(ctx, prefixKeys(uniqueVisitorsPrefix, days)...).Result()
}

// CountCalendars returns the number of unique calendars fetched across the
// given days.
func (c *RedisCache) CountCalendars(ctx context.Context, days []string) (int64, error) {
	return c.client.PFCount(ctx, prefixKeys(uniqueCalendarsPrefix, days)...).Result()
}

func (c *RedisCache) addUnique(ctx context.Context, key, member string) error {
	pipe := c.client.Pipeline()
	pipe.PFAdd(ctx, key, member)
	pipe.Expire(ctx, key, uniqueKeyTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func prefixKeys(prefix string, days []string) []string {
	keys := make([]string, len(days))
	for i, day := range days {
		keys[i] = prefix + day
	}
	return keys
}

// lastDays returns the n most recent UTC days ending at now, as key suffixes.
func lastDays(now time.Time, n int) []string {
	days := make([]string, n)
	for i := range days {
		days[i] = now.UTC().AddDate(0, 0, -i).Format(uniqueDayFormat)
	}
	return days
}

// Telemetry records request metrics through OpenTelemetry.
type Telemetry struct {
	requests metric.Int64Counter
	uniques  UniqueStore
	logger   *zap.Logger
}

// NewTelemetry creates the request counter plus, for each non-nil source,
// gauges for unique visitors/calendars (from Redis) and for what active
// calendars track (from the database). Every replica reports the same gauge
// values, so dashboards should aggregate them with max(), not sum().
func NewTelemetry(
	meter metric.Meter,
	uniques UniqueStore,
	tracked TrackedSource,
	logger *zap.Logger,
) (*Telemetry, error) {
	requests, err := meter.Int64Counter(telemetryMeterPrefix+"http.requests",
		metric.WithDescription("HTTP requests served, by route, method and status."),
		metric.WithUnit("{request}"))
	if err != nil {
		return nil, fmt.Errorf("failed to create request counter: %w", err)
	}

	if tracked != nil {
		if trackedErr := registerTrackedGauges(meter, tracked, logger); trackedErr != nil {
			return nil, trackedErr
		}
	}

	t := &Telemetry{requests: requests, uniques: uniques, logger: logger}
	if uniques == nil {
		return t, nil
	}

	visitors, err := meter.Int64ObservableGauge(telemetryMeterPrefix+"visitors.unique",
		metric.WithDescription("Unique visitors (hashed IP + User-Agent) in the window."),
		metric.WithUnit("{visitor}"))
	if err != nil {
		return nil, fmt.Errorf("failed to create unique visitors gauge: %w", err)
	}
	calendars, err := meter.Int64ObservableGauge(telemetryMeterPrefix+"calendars.unique",
		metric.WithDescription("Unique calendar feeds fetched in the window."),
		metric.WithUnit("{calendar}"))
	if err != nil {
		return nil, fmt.Errorf("failed to create unique calendars gauge: %w", err)
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		now := time.Now()
		for _, w := range uniqueWindows {
			days := lastDays(now, w.days)
			attrs := metric.WithAttributes(attribute.String("window", w.name))
			// On error, skip the observation rather than report a false 0.
			if n, countErr := uniques.CountVisitors(ctx, days); countErr == nil {
				o.ObserveInt64(visitors, n, attrs)
			} else {
				logger.Debug("Failed to count unique visitors", zap.Error(countErr))
			}
			if n, countErr := uniques.CountCalendars(ctx, days); countErr == nil {
				o.ObserveInt64(calendars, n, attrs)
			} else {
				logger.Debug("Failed to count unique calendars", zap.Error(countErr))
			}
		}
		return nil
	}, visitors, calendars)
	if err != nil {
		return nil, fmt.Errorf("failed to register unique count callback: %w", err)
	}
	return t, nil
}

// Middleware counts each request and records unique visitors/calendars.
// Static assets, robots.txt and sitemap.xml are ignored. Unmatched routes are
// counted under a fixed route label but not as visitors. Successful .ics
// fetches are tracked per calendar hash rather than per visitor, because
// calendar apps poll from shared fetcher IPs. Unique recording runs in the
// background so Redis latency never delays a response.
func (t *Telemetry) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/static/") || path == "/robots.txt" || path == "/sitemap.xml" {
			return
		}

		status := c.Writer.Status()
		route := c.FullPath()
		var record func(ctx context.Context, day string) error
		switch {
		case strings.HasSuffix(path, ".ics"):
			// Never label by the raw path: each calendar hash would become
			// its own metric series.
			route = calendarRoute
			if status == http.StatusOK {
				hash := strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".ics")
				record = func(ctx context.Context, day string) error {
					return t.uniques.AddCalendar(ctx, day, hash)
				}
			}
		case route == "":
			route = unmatchedRoute
		default:
			visitor := visitorID(c)
			record = func(ctx context.Context, day string) error {
				return t.uniques.AddVisitor(ctx, day, visitor)
			}
		}

		t.requests.Add(c.Request.Context(), 1, metric.WithAttributes(
			semconv.HTTPRoute(route),
			semconv.HTTPRequestMethodKey.String(c.Request.Method),
			semconv.HTTPResponseStatusCode(status),
		))

		if record == nil || t.uniques == nil {
			return
		}
		day := time.Now().UTC().Format(uniqueDayFormat)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), uniqueRecordTimeout)
			defer cancel()
			if err := record(ctx, day); err != nil {
				t.logger.Debug("Failed to record unique", zap.Error(err), zap.String("path", path))
			}
		}()
	}
}

// visitorID returns an anonymous identifier for the client: a hash of the IP
// and User-Agent. The raw IP is never stored.
func visitorID(c *gin.Context) string {
	ip := c.GetHeader("CF-Connecting-IP")
	if ip == "" {
		ip = c.ClientIP()
	}
	sum := sha256.Sum256([]byte(ip + "\x00" + c.Request.UserAgent()))
	return hex.EncodeToString(sum[:visitorIDBytes])
}
