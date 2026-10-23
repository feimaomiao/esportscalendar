package middleware

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

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
			ics := generateICS([]dbtypes.GetCalendarMatchesBySelectionsRow{testMatch(tt.streamURL)}, false, "https://esportscalendar.app")
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
	ics := generateICS([]dbtypes.GetCalendarMatchesBySelectionsRow{testMatch("https://www.twitch.tv/riotgames")}, false, "https://esportscalendar.app")
	if !strings.HasSuffix(ics, "END:VCALENDAR\r\n") {
		t.Fatalf("calendar does not end with END:VCALENDAR CRLF")
	}
	for _, line := range strings.Split(strings.TrimSuffix(ics, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Errorf("line exceeds 75 octets (%d): %q", len(line), line)
		}
	}
	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	if !strings.Contains(unfolded, "DESCRIPTION:Cloud9 vs Karmine Corp ÆØÅ [0-0] - Play-In - Worlds (LoL)\\n\\nWatch live: https://www.twitch.tv/riotgames\r\n") {
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
