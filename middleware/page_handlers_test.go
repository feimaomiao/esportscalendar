package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestMonthAnchor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		year  int
		month int
		want  time.Time
	}{
		{"january", 2025, 1, time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)},
		{"december", 2025, 12, time.Date(2025, time.December, 1, 0, 0, 0, 0, time.UTC)},
		{"leap-feb", 2024, 2, time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := monthAnchor(tt.year, tt.month)
			if !got.Equal(tt.want) {
				t.Fatalf("monthAnchor(%d,%d) = %v, want %v", tt.year, tt.month, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("monthAnchor(%d,%d) location = %v, want UTC", tt.year, tt.month, got.Location())
			}
		})
	}
}

func TestAddMonths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		year      int
		month     int
		delta     int
		wantYear  int
		wantMonth int
	}{
		{"zero delta", 2025, 6, 0, 2025, 6},
		{"forward within year", 2025, 6, 3, 2025, 9},
		{"forward wraps to next year", 2025, 11, 3, 2026, 2},
		{"backward within year", 2025, 6, -2, 2025, 4},
		{"backward wraps to previous year", 2025, 2, -3, 2024, 11},
		{"twelve forward keeps month, advances year", 2025, 5, 12, 2026, 5},
		{"twenty-four backward jumps two years", 2025, 5, -24, 2023, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			y, m := addMonths(tt.year, tt.month, tt.delta)
			if y != tt.wantYear || m != tt.wantMonth {
				t.Fatalf("addMonths(%d,%d,%d) = (%d,%d), want (%d,%d)",
					tt.year, tt.month, tt.delta, y, m, tt.wantYear, tt.wantMonth)
			}
			if m < 1 || m > 12 {
				t.Fatalf("addMonths returned month %d outside [1,12]", m)
			}
		})
	}
}

func TestCalendarMonthBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		year      int
		month     int
		wantStart time.Time
		wantEnd   time.Time
	}{
		// Bounds are padded by ±1 UTC day so client-side local-date rebucketing
		// in calendar.js can pick up matches that bleed across month boundaries
		// in non-UTC viewer timezones (see calendarMonthBounds godoc).
		{
			name: "regular month",
			year: 2025, month: 6,
			wantStart: time.Date(2025, time.May, 31, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.July, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "december wraps end to january next year",
			year: 2025, month: 12,
			wantStart: time.Date(2025, time.November, 30, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "leap february",
			year: 2024, month: 2,
			wantStart: time.Date(2024, time.January, 31, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, time.March, 2, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start, end := calendarMonthBounds(tt.year, tt.month)
			if !start.Equal(tt.wantStart) {
				t.Errorf("start = %v, want %v", start, tt.wantStart)
			}
			if !end.Equal(tt.wantEnd) {
				t.Errorf("end = %v, want %v", end, tt.wantEnd)
			}
			if !end.After(start) {
				t.Errorf("end (%v) must be strictly after start (%v)", end, start)
			}
		})
	}
}

func TestClampCalendarMonth(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC)
	// Upper bound from now: 2026-05 + 12 months = 2027-05.
	const wantMaxYear, wantMaxMonth = 2027, 5

	tests := []struct {
		name      string
		year      int
		month     int
		wantYear  int
		wantMonth int
	}{
		{
			name: "in-range month passes through",
			year: 2026, month: 1,
			wantYear: 2026, wantMonth: 1,
		},
		{
			name: "exactly at lower bound passes through",
			year: calendarMinYear, month: calendarMinMonth,
			wantYear: calendarMinYear, wantMonth: calendarMinMonth,
		},
		{
			name: "below lower bound clamps up",
			year: 2024, month: 12,
			wantYear: calendarMinYear, wantMonth: calendarMinMonth,
		},
		{
			name: "year well below lower bound clamps up",
			year: 1999, month: 1,
			wantYear: calendarMinYear, wantMonth: calendarMinMonth,
		},
		{
			name: "exactly at upper bound passes through",
			year: wantMaxYear, month: wantMaxMonth,
			wantYear: wantMaxYear, wantMonth: wantMaxMonth,
		},
		{
			name: "above upper bound clamps down",
			year: 2030, month: 1,
			wantYear: wantMaxYear, wantMonth: wantMaxMonth,
		},
		{
			name: "current month passes through",
			year: now.Year(), month: int(now.Month()),
			wantYear: now.Year(), wantMonth: int(now.Month()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			y, m := clampCalendarMonth(tt.year, tt.month, now)
			if y != tt.wantYear || m != tt.wantMonth {
				t.Fatalf("clampCalendarMonth(%d,%d, %v) = (%d,%d), want (%d,%d)",
					tt.year, tt.month, now, y, m, tt.wantYear, tt.wantMonth)
			}
		})
	}
}

func TestDefaultCalendarMonth(t *testing.T) {
	t.Parallel()

	t.Run("now in range returns now's month", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC)
		y, m := defaultCalendarMonth(now)
		if y != 2026 || m != 5 {
			t.Fatalf("got (%d,%d), want (2026,5)", y, m)
		}
	})

	t.Run("now before lower bound clamps to lower bound", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
		y, m := defaultCalendarMonth(now)
		if y != calendarMinYear || m != calendarMinMonth {
			t.Fatalf("got (%d,%d), want (%d,%d)", y, m, calendarMinYear, calendarMinMonth)
		}
	})

	t.Run("now exactly at lower bound returns it", func(t *testing.T) {
		t.Parallel()
		now := time.Date(calendarMinYear, time.Month(calendarMinMonth), 15, 12, 0, 0, 0, time.UTC)
		y, m := defaultCalendarMonth(now)
		if y != calendarMinYear || m != calendarMinMonth {
			t.Fatalf("got (%d,%d), want (%d,%d)", y, m, calendarMinYear, calendarMinMonth)
		}
	})
}

func TestIsTier1League(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   any
		want bool
	}{
		{"nil is not tier 1", nil, false},
		{"int32(1) is tier 1", int32(1), true},
		{"int32(2) is not tier 1", int32(2), false},
		{"int32(0) is not tier 1", int32(0), false},
		{"int64(1) is tier 1", int64(1), true},
		{"int64(2) is not tier 1", int64(2), false},
		{"int(1) untyped goes through default branch", 1, false},
		{"string '1' is not tier 1", "1", false},
		{"float64(1) is not tier 1", float64(1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isTier1League(tt.in); got != tt.want {
				t.Fatalf("isTier1League(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsHTMXRequest(t *testing.T) {
	t.Parallel()

	t.Run("returns true when Hx-Request: true", func(t *testing.T) {
		t.Parallel()
		c, _ := newTestContext(t, http.MethodGet, "/", nil)
		c.Request.Header.Set("Hx-Request", "true")
		if !isHTMXRequest(c) {
			t.Fatal("expected true")
		}
	})

	t.Run("returns false when header missing", func(t *testing.T) {
		t.Parallel()
		c, _ := newTestContext(t, http.MethodGet, "/", nil)
		if isHTMXRequest(c) {
			t.Fatal("expected false")
		}
	})

	t.Run("returns false for any other value", func(t *testing.T) {
		t.Parallel()
		c, _ := newTestContext(t, http.MethodGet, "/", nil)
		c.Request.Header.Set("Hx-Request", "false")
		if isHTMXRequest(c) {
			t.Fatal("expected false")
		}
	})
}

func TestSetHTMXTitle(t *testing.T) {
	t.Parallel()

	c, w := newTestContext(t, http.MethodGet, "/", nil)
	setHTMXTitle(c, "Calendar - EsportsCalendar")

	got := w.Header().Get("HX-Trigger")
	if got == "" {
		t.Fatal("HX-Trigger header was not set")
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("HX-Trigger payload not valid JSON: %v (%q)", err, got)
	}
	inner, ok := payload["page:title"]
	if !ok {
		t.Fatalf("payload missing page:title key: %q", got)
	}
	if inner["title"] != "Calendar - EsportsCalendar" {
		t.Fatalf("title = %q, want %q", inner["title"], "Calendar - EsportsCalendar")
	}
}

func newTestContext(t *testing.T, method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, path, nil)
	_ = body // body unused for the cases we cover; keep param for future expansion
	c.Request = req
	return c, w
}
