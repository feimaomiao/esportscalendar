package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v4"
	"go.uber.org/zap/zaptest"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

func sampleMatch() dbtypes.GetCalendarMatchesBySelectionsRow {
	return dbtypes.GetCalendarMatchesBySelectionsRow{
		Name:           "KT vs T1",
		GameName:       "LoL",
		TournamentName: "Round 3-4 Legend Group",
		Team1Name:      text("KT Rolster"),
		Team1Acronym:   text("KT"),
		Team2Name:      text("T1"),
		Team2Acronym:   text("T1"),
		Team1Score:     1,
		Team2Score:     2,
	}
}

func TestParseTitleFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   any
		want titleFormat
	}{
		{"missing", nil, titleFormatDefault},
		{"default", "default", titleFormatDefault},
		{"teams first", "teams-first", titleFormatTeamsFirst},
		{"teams only", "teams-only", titleFormatTeamsOnly},
		{"unknown string", "something-else", titleFormatDefault},
		{"wrong type", true, titleFormatDefault},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := parseTitleFormat(tc.in); got != tc.want {
				t.Fatalf("parseTitleFormat(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildSummary(t *testing.T) {
	t.Parallel()

	finished := sampleMatch()
	finished.Finished = true

	noTournament := sampleMatch()
	noTournament.TournamentName = ""

	oneTeam := sampleMatch()
	oneTeam.Team2Name = pgtype.Text{}
	oneTeam.Team2Acronym = pgtype.Text{}

	noAcronym := sampleMatch()
	noAcronym.Team1Acronym = pgtype.Text{}

	noTeams := sampleMatch()
	noTeams.Name = "Winner of A vs Winner of B"
	noTeams.Team1Name, noTeams.Team1Acronym = pgtype.Text{}, pgtype.Text{}
	noTeams.Team2Name, noTeams.Team2Acronym = pgtype.Text{}, pgtype.Text{}

	tests := []struct {
		name       string
		match      dbtypes.GetCalendarMatchesBySelectionsRow
		hideScores bool
		format     titleFormat
		want       string
	}{
		{"default", sampleMatch(), false, titleFormatDefault, "[LoL] Round 3-4 Legend Group - KT vs T1"},
		{"default finished", finished, false, titleFormatDefault, "[LoL] Round 3-4 Legend Group - KT vs T1 [1-2]"},
		{"default finished hidden", finished, true, titleFormatDefault, "[LoL] Round 3-4 Legend Group - KT vs T1"},
		{"default no tournament", noTournament, false, titleFormatDefault, "[LoL] KT vs T1"},
		{"teams first", sampleMatch(), false, titleFormatTeamsFirst, "KT vs T1 - Round 3-4 Legend Group [LoL]"},
		{
			"teams first finished",
			finished,
			false,
			titleFormatTeamsFirst,
			"KT vs T1 [1-2] - Round 3-4 Legend Group [LoL]",
		},
		{"teams first hidden", finished, true, titleFormatTeamsFirst, "KT vs T1 - Round 3-4 Legend Group [LoL]"},
		{"teams first no tournament", noTournament, false, titleFormatTeamsFirst, "KT vs T1 [LoL]"},
		{"teams only", sampleMatch(), false, titleFormatTeamsOnly, "KT vs T1"},
		{"teams only finished", finished, false, titleFormatTeamsOnly, "KT vs T1 [1-2]"},
		{"teams only hidden", finished, true, titleFormatTeamsOnly, "KT vs T1"},
		{"one team TBD", oneTeam, false, titleFormatTeamsOnly, "KT vs TBD"},
		{"name fallback without acronym", noAcronym, false, titleFormatTeamsOnly, "KT Rolster vs T1"},
		{"no teams uses match name", noTeams, false, titleFormatTeamsOnly, "Winner of A vs Winner of B"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := buildSummary(tc.match, tc.hideScores, tc.format); got != tc.want {
				t.Fatalf("buildSummary() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenerateICSUsesTitleFormat(t *testing.T) {
	t.Parallel()

	match := sampleMatch()
	match.ExpectedStartTime = pgtype.Timestamp{Valid: true}
	matches := []dbtypes.GetCalendarMatchesBySelectionsRow{match}

	ics := generateICS(matches, false, titleFormatTeamsOnly, "https://example.com")
	if !strings.Contains(ics, "SUMMARY:KT vs T1\r\n") {
		t.Fatalf("expected teams-only summary in ICS, got:\n%s", ics)
	}
}

func TestExportHandlerStoresTitleFormat(t *testing.T) {
	const selections = `"selections":{"1":{"leagues":[10]}},"hideScores":true`

	tests := []struct {
		name   string
		body   string
		stored string
	}{
		{
			// Default/omitted formats must keep the pre-existing payload so
			// existing selections hash to the same calendar URL.
			name:   "omitted",
			body:   `{` + selections + `}`,
			stored: `{"hideScores":true,"selections":{"1":{"leagues":[10]}}}`,
		},
		{
			name:   "explicit default is dropped",
			body:   `{` + selections + `,"titleFormat":"default"}`,
			stored: `{"hideScores":true,"selections":{"1":{"leagues":[10]}}}`,
		},
		{
			name:   "unknown value is dropped",
			body:   `{` + selections + `,"titleFormat":"<script>"}`,
			stored: `{"hideScores":true,"selections":{"1":{"leagues":[10]}}}`,
		},
		{
			name:   "teams only is kept",
			body:   `{` + selections + `,"titleFormat":"teams-only"}`,
			stored: `{"hideScores":true,"selections":{"1":{"leagues":[10]}},"titleFormat":"teams-only"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("pgxmock.NewPool: %v", err)
			}
			t.Cleanup(func() { mockDB.Close() })

			mw := &Middleware{
				DBConn:  dbtypes.New(mockDB),
				Context: context.Background(),
				Logger:  zaptest.NewLogger(t),
				BaseURL: "https://test.example",
			}
			mockDB.ExpectExec(`INSERT INTO url_mappings`).
				WithArgs(CalendarHash([]byte(tc.stored)), []byte(tc.stored)).
				WillReturnResult(pgxmock.NewResult("INSERT", 1))

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/export", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")

			mw.ExportHandler(c)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
			}
			if err := mockDB.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet DB expectations: %v", err)
			}
		})
	}
}

func testMatch(streamURL string) dbtypes.GetCalendarMatchesBySelectionsRow {
	return dbtypes.GetCalendarMatchesBySelectionsRow{
		ID:                1,
		Name:              "Upper Bracket Semifinal 2: C9 vs KC",
		ExpectedStartTime: pgtype.Timestamp{Time: time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC), Valid: true},
		Finished:          true,
		AmountOfGames:     3,
		StreamURL:         pgtype.Text{String: streamURL, Valid: streamURL != ""},
		GameName:          "LoL",
		LeagueName:        "Worlds",
		TournamentName:    "Play-In",
		Team1Name:         pgtype.Text{String: "Cloud9", Valid: true},
		Team2Name:         pgtype.Text{String: "Karmine Corp ÆØÅ", Valid: true},
	}
}

func TestGenerateICSSingleURL(t *testing.T) {
	tests := []struct {
		name      string
		streamURL string
		wantURL   string
	}{
		{"with stream", "https://www.twitch.tv/riotgames", "URL:https://www.twitch.tv/riotgames"},
		{"without stream", "", "URL:https://esportscalendar.app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ics := generateICS(
				[]dbtypes.GetCalendarMatchesBySelectionsRow{testMatch(tt.streamURL)},
				false,
				titleFormatDefault,
				"https://esportscalendar.app",
			)
			var urls []string
			for _, line := range strings.Split(ics, "\r\n") {
				if strings.HasPrefix(line, "URL:") {
					urls = append(urls, line)
				}
			}
			if len(urls) != 1 || urls[0] != tt.wantURL {
				t.Errorf("got URL lines %q, want exactly [%q]", urls, tt.wantURL)
			}
		})
	}
}

func TestGenerateICSFoldsLongLines(t *testing.T) {
	ics := generateICS(
		[]dbtypes.GetCalendarMatchesBySelectionsRow{testMatch("https://www.twitch.tv/riotgames")},
		false,
		titleFormatDefault,
		"https://esportscalendar.app",
	)
	if !strings.HasSuffix(ics, "END:VCALENDAR\r\n") {
		t.Fatalf("calendar does not end with END:VCALENDAR CRLF")
	}
	for _, line := range strings.Split(strings.TrimSuffix(ics, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Errorf("line exceeds 75 octets (%d): %q", len(line), line)
		}
	}
	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	if !strings.Contains(
		unfolded,
		"DESCRIPTION:Cloud9 vs Karmine Corp ÆØÅ [0-0] - Play-In - Worlds (LoL)\\n\\nWatch live: https://www.twitch.tv/riotgames\r\n",
	) {
		t.Errorf("unfolded DESCRIPTION does not round-trip:\n%s", unfolded)
	}
}

func TestFoldICSKeepsMultiByteRunesIntact(t *testing.T) {
	line := "SUMMARY:" + strings.Repeat("Ø", 60)
	folded := foldICS(line + "\r\n")
	for _, part := range strings.Split(strings.TrimSuffix(folded, "\r\n"), "\r\n") {
		if len(part) > 75 {
			t.Errorf("folded line exceeds 75 octets: %d", len(part))
		}
		if !strings.HasPrefix(part, "SUMMARY:") && !strings.HasPrefix(part, " Ø") {
			t.Errorf("continuation line split a rune: %q", part)
		}
	}
	if strings.ReplaceAll(folded, "\r\n ", "") != line+"\r\n" {
		t.Errorf("folding is not reversible")
	}
}

func TestGenerateExpiredICS(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC)
	got := generateExpiredICS("0123456789abcdef", "https://test.example", since)
	unfolded := strings.ReplaceAll(got, "\r\n ", "")
	for _, want := range []string{
		"X-WR-CALNAME:Esports Calendar (link expired)\r\n",
		"UID:expired-0123456789abcdef@https://test.example\r\n",
		"DTSTART;VALUE=DATE:20261102\r\n",
		"DTEND;VALUE=DATE:20261103\r\n",
		"RRULE:FREQ=DAILY;COUNT=30\r\n",
		"URL:https://test.example/calendar\r\n",
		"https://test.example/calendar\\, add the new link",
	} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("notice missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "BEGIN:VEVENT") != 1 {
		t.Errorf("want exactly one event:\n%s", got)
	}
	if again := generateExpiredICS("0123456789abcdef", "https://test.example", since); again != got {
		t.Error("notice must be identical across polls so clients update rather than duplicate it")
	}
	for _, line := range strings.Split(got, "\r\n") {
		if len(line) > 75 {
			t.Errorf("unfolded line over 75 octets: %q", line)
		}
	}
}

func TestCalendarICSHandlerMissingCalendars(t *testing.T) {
	t.Parallel()
	const known = "0123456789abcdef"
	tests := []struct {
		name       string
		path       string
		setup      func(mock pgxmock.PgxPoolIface)
		wantStatus int
		wantNotice bool
	}{
		{
			name: "lost hash gets the notice and is recorded",
			path: "/" + known + ".ics",
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`FROM url_mappings`).WithArgs(known).WillReturnError(pgx.ErrNoRows)
				mock.ExpectQuery(`INSERT INTO missing_calendars`).WithArgs(known, int64(maxMissingCalendars)).
					WillReturnRows(pgxmock.NewRows([]string{"first_seen"}).AddRow(
						pgtype.Timestamp{Time: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC), Valid: true}))
			},
			wantStatus: http.StatusOK,
			wantNotice: true,
		},
		{
			name: "notice is still served when recording fails",
			path: "/" + known + ".ics",
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`FROM url_mappings`).WithArgs(known).WillReturnError(pgx.ErrNoRows)
				mock.ExpectQuery(`INSERT INTO missing_calendars`).WithArgs(known, int64(maxMissingCalendars)).
					WillReturnError(errors.New("relation does not exist"))
			},
			wantStatus: http.StatusOK,
			wantNotice: true,
		},
		{
			name: "malformed hash is a plain 404 and not recorded",
			path: "/wp-admin.ics",
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`FROM url_mappings`).WithArgs("wp-admin").WillReturnError(pgx.ErrNoRows)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "database failure is a 503, not a dead calendar",
			path: "/" + known + ".ics",
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`FROM url_mappings`).WithArgs(known).WillReturnError(errors.New("connection refused"))
			},
			wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mockDB, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("pgxmock.NewPool: %v", err)
			}
			t.Cleanup(func() { mockDB.Close() })
			tc.setup(mockDB)

			mw := &Middleware{
				DBConn:  dbtypes.New(mockDB),
				Context: context.Background(),
				Logger:  zaptest.NewLogger(t),
				BaseURL: "https://test.example",
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)

			mw.CalendarICSHandler(c)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			isNotice := strings.Contains(w.Body.String(), "UID:expired-"+known)
			if isNotice != tc.wantNotice {
				t.Errorf("notice served = %v, want %v, body=%s", isNotice, tc.wantNotice, w.Body.String())
			}
			if c.GetBool(calendarExpiredKey) != tc.wantNotice {
				t.Errorf("expired flag = %v, want %v", c.GetBool(calendarExpiredKey), tc.wantNotice)
			}
			if tc.wantNotice {
				if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
					t.Errorf("Cache-Control = %q, want no-store", cc)
				}
				if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
					t.Errorf("Content-Type = %q, want text/calendar", ct)
				}
			}
			if err := mockDB.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet DB expectations: %v", err)
			}
		})
	}
}

func TestGenerateICSStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status pgtype.Text
		want   string
	}{
		{name: "not yet refetched", status: pgtype.Text{}, want: "STATUS:CONFIRMED"},
		{name: "not started", status: text("not_started"), want: "STATUS:CONFIRMED"},
		{name: "canceled", status: text("canceled"), want: "STATUS:CANCELLED"},
		{name: "removed upstream", status: text("removed"), want: "STATUS:CANCELLED"},
		{name: "postponed", status: text("postponed"), want: "STATUS:TENTATIVE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			match := testMatch("")
			match.Finished = false
			match.Status = tc.status
			out := generateICS(
				[]dbtypes.GetCalendarMatchesBySelectionsRow{match},
				false,
				titleFormatDefault,
				"http://test",
			)
			if !strings.Contains(out, tc.want+"\r\n") {
				t.Fatalf("output has no %s:\n%s", tc.want, out)
			}
		})
	}
}

// A match that was never played must not show a 0-0 result, even if a stale
// finished flag says otherwise.
func TestGenerateICSCancelledHasNoScore(t *testing.T) {
	t.Parallel()
	match := sampleMatch()
	match.ExpectedStartTime = pgtype.Timestamp{Time: time.Now().Add(-time.Hour), Valid: true}
	match.Finished = true
	match.Status = text("canceled")
	if got := buildSummary(match, false, titleFormatTeamsOnly); strings.Contains(got, "[1-2]") {
		t.Fatalf("cancelled match shows a score: %q", got)
	}

	match.Status = text("finished")
	if got := buildSummary(match, false, titleFormatTeamsOnly); !strings.Contains(got, "[1-2]") {
		t.Fatalf("finished match lost its score: %q", got)
	}
}

func TestGenerateICSVersioning(t *testing.T) {
	t.Parallel()
	generate := func(match dbtypes.GetCalendarMatchesBySelectionsRow) string {
		return generateICS([]dbtypes.GetCalendarMatchesBySelectionsRow{match}, false, titleFormatDefault, "http://test")
	}

	t.Run("stamped with the last change", func(t *testing.T) {
		t.Parallel()
		match := testMatch("")
		match.ModifiedAt = pgtype.Timestamp{Time: time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC), Valid: true}
		match.Revision = 4
		out := generate(match)
		for _, want := range []string{
			"DTSTAMP:20261001T093000Z\r\n", "LAST-MODIFIED:20261001T093000Z\r\n", "SEQUENCE:4\r\n",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output has no %q:\n%s", want, out)
			}
		}
	})

	t.Run("rows without modified_at keep a valid stamp", func(t *testing.T) {
		t.Parallel()
		out := generate(testMatch(""))
		if !strings.Contains(out, "DTSTAMP:20261015T180000Z\r\n") || !strings.Contains(out, "SEQUENCE:0\r\n") {
			t.Errorf("unexpected stamp:\n%s", out)
		}
		if strings.Contains(out, "LAST-MODIFIED") {
			t.Errorf("LAST-MODIFIED with nothing to report:\n%s", out)
		}
	})
}

func TestGenerateICSForfeit(t *testing.T) {
	t.Parallel()
	match := testMatch("")
	match.Finished = false
	match.Status = text("canceled")
	match.Forfeit = true
	match.Team1ID, match.Team2ID, match.WinnerID = 7, 8, 7
	rows := []dbtypes.GetCalendarMatchesBySelectionsRow{match}

	shown := generateICS(rows, false, titleFormatDefault, "http://test")
	if !strings.Contains(strings.ReplaceAll(shown, "\r\n ", ""), "Cloud9 won by forfeit") {
		t.Errorf("winner missing:\n%s", shown)
	}
	if !strings.Contains(shown, "STATUS:CANCELLED\r\n") {
		t.Errorf("forfeit should be cancelled, it was never played:\n%s", shown)
	}
	hidden := generateICS(rows, true, titleFormatDefault, "http://test")
	if strings.Contains(hidden, "won by forfeit") {
		t.Errorf("hideScores leaked the winner:\n%s", hidden)
	}
}
