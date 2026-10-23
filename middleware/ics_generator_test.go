package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
				WithArgs(generateHash([]byte(tc.stored)), []byte(tc.stored)).
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
