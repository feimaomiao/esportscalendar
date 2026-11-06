package middleware

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/feimaomiao/esportscalendar/components"
	"github.com/feimaomiao/esportscalendar/dbtypes"
)

// titleFormat controls how the SUMMARY (event title) of each match is laid out.
type titleFormat string

const (
	// titleFormatDefault: [Game] Tournament - Match Name.
	titleFormatDefault titleFormat = "default"
	// titleFormatTeamsFirst: Team1 vs Team2 - Tournament [Game].
	titleFormatTeamsFirst titleFormat = "teams-first"
	// titleFormatTeamsOnly: Team1 vs Team2.
	titleFormatTeamsOnly titleFormat = "teams-only"
)

// unknownTeam is shown in place of a team that has not been decided yet.
const unknownTeam = "TBD"

// parseTitleFormat maps a stored/requested value to a known titleFormat,
// falling back to the default for missing or unknown values.
func parseTitleFormat(v any) titleFormat {
	s, _ := v.(string)
	if f := titleFormat(s); f == titleFormatTeamsFirst || f == titleFormatTeamsOnly {
		return f
	}
	return titleFormatDefault
}

// teamLabel prefers the short acronym (fits small calendar widgets), then the full name.
func teamLabel(acronym, name pgtype.Text) string {
	if acronym.Valid && acronym.String != "" {
		return acronym.String
	}
	if name.Valid && name.String != "" {
		return name.String
	}
	return unknownTeam
}

// icsMatchState derives a feed row's state, so the feed can't disagree with the
// pages about what happened to a match.
func icsMatchState(match dbtypes.GetCalendarMatchesBySelectionsRow) components.MatchState {
	return components.DeriveMatchState(components.MatchFacts{
		Status:   match.Status,
		Finished: match.Finished,
		IsLive:   match.IsLive,
		Forfeit:  match.Forfeit,
		Start:    match.ExpectedStartTime,
	}, time.Now())
}

// icsStamp is the event's last change. Rows stalka hasn't refetched since
// modified_at was added have no such time, so they fall back to the start time.
func icsStamp(match dbtypes.GetCalendarMatchesBySelectionsRow) time.Time {
	if match.ModifiedAt.Valid {
		return match.ModifiedAt.Time
	}
	return match.ExpectedStartTime.Time
}

// icsStatus maps a match state to the VEVENT STATUS property.
func icsStatus(state components.MatchState) string {
	switch state {
	case components.MatchCancelled, components.MatchForfeit:
		return "CANCELLED"
	case components.MatchPostponed:
		return "TENTATIVE"
	case components.MatchUpcoming, components.MatchOngoing,
		components.MatchAwaitingResult, components.MatchFinished:
		return "CONFIRMED"
	}
	return "CONFIRMED"
}

func buildSummary(match dbtypes.GetCalendarMatchesBySelectionsRow, hideScores bool, format titleFormat) string {
	score := ""
	if icsMatchState(match) == components.MatchFinished && !hideScores {
		score = fmt.Sprintf(" [%d-%d]", match.Team1Score, match.Team2Score)
	}

	if format == titleFormatDefault {
		// [Game] Tournament - Match Name (omit tournament if empty)
		summary := fmt.Sprintf("[%s] %s", match.GameName, match.Name)
		if match.TournamentName != "" {
			summary = fmt.Sprintf("[%s] %s - %s", match.GameName, match.TournamentName, match.Name)
		}
		return summary + score
	}

	// Use the match name when neither team is known yet, since it usually
	// carries more context (e.g. "Winner of A vs B") than "TBD vs TBD".
	matchup := match.Name
	if match.Team1Name.Valid || match.Team2Name.Valid {
		matchup = fmt.Sprintf("%s vs %s",
			teamLabel(match.Team1Acronym, match.Team1Name),
			teamLabel(match.Team2Acronym, match.Team2Name),
		)
	}
	matchup += score

	if format == titleFormatTeamsOnly {
		return matchup
	}
	if match.TournamentName != "" {
		return fmt.Sprintf("%s - %s [%s]", matchup, match.TournamentName, match.GameName)
	}
	return fmt.Sprintf("%s [%s]", matchup, match.GameName)
}

func generateICS(
	matches []dbtypes.GetCalendarMatchesBySelectionsRow,
	hideScores bool,
	format titleFormat,
	baseURL string,
) string {
	var ics strings.Builder

	ics.WriteString("BEGIN:VCALENDAR\r\n")
	ics.WriteString("VERSION:2.0\r\n")
	ics.WriteString("PRODID:-//EsportsCalendar//EN\r\n")
	ics.WriteString("CALSCALE:GREGORIAN\r\n")
	ics.WriteString("X-WR-CALNAME:Esports Calendar\r\n")
	ics.WriteString("X-WR-TIMEZONE:UTC\r\n")

	for _, match := range matches {
		// Get team names with fallback to "TBD"
		team1Name := unknownTeam
		if match.Team1Name.Valid {
			team1Name = match.Team1Name.String
		}
		team2Name := unknownTeam
		if match.Team2Name.Valid {
			team2Name = match.Team2Name.String
		}
		if !match.ExpectedStartTime.Valid {
			continue
		}

		startTime := match.ExpectedStartTime.Time
		// Calculate duration: 1 hour per game
		duration := time.Duration(match.AmountOfGames) * time.Hour
		endTime := startTime.Add(duration)

		ics.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&ics, "UID:%d@%s\r\n", match.ID, baseURL)
		fmt.Fprintf(&ics, "DTSTAMP:%s\r\n", icsStamp(match).UTC().Format("20060102T150405Z"))
		if match.ModifiedAt.Valid {
			fmt.Fprintf(&ics, "LAST-MODIFIED:%s\r\n", match.ModifiedAt.Time.UTC().Format("20060102T150405Z"))
		}
		fmt.Fprintf(&ics, "SEQUENCE:%d\r\n", match.Revision)
		fmt.Fprintf(&ics, "DTSTART:%s\r\n", startTime.UTC().Format("20060102T150405Z"))
		fmt.Fprintf(&ics, "DTEND:%s\r\n", endTime.UTC().Format("20060102T150405Z"))

		// Scores are appended to the title if match is finished and hideScores is false
		fmt.Fprintf(&ics, "SUMMARY:%s\r\n", escapeICS(buildSummary(match, hideScores, format)))

		// Build description with teams, league, tournament, and score for finished matches
		description := fmt.Sprintf("%s vs %s - %s - %s (%s)",
			team1Name,
			team2Name,
			match.TournamentName,
			match.LeagueName,
			match.GameName,
		)
		state := icsMatchState(match)
		if state == components.MatchFinished {
			if hideScores {
				// Show "Finished" instead of score
				description = fmt.Sprintf("%s vs %s [Finished] - %s - %s (%s)",
					team1Name,
					team2Name,
					match.TournamentName,
					match.LeagueName,
					match.GameName,
				)
			} else {
				description = fmt.Sprintf("%s vs %s [%d-%d] - %s - %s (%s)",
					team1Name,
					team2Name,
					match.Team1Score,
					match.Team2Score,
					match.TournamentName,
					match.LeagueName,
					match.GameName,
				)
			}
		}
		if state == components.MatchForfeit && !hideScores {
			winner := components.ForfeitWinner(
				match.WinnerID, match.Team1ID, match.Team2ID, match.Team1Name, match.Team2Name,
			)
			if winner != "" {
				description += "\n\n" + winner + " won by forfeit"
			}
		}
		// Add stream URL to description if available
		if match.StreamURL.Valid && match.StreamURL.String != "" {
			description += "\n\nWatch live: " + match.StreamURL.String
		}
		fmt.Fprintf(&ics, "DESCRIPTION:%s\r\n", escapeICS(description))

		// Emit standard iCalendar URL property so calendar clients render a clickable link.
		// RFC 5545 allows URL at most once per VEVENT; Outlook rejects the feed otherwise.
		eventURL := baseURL
		if match.StreamURL.Valid && match.StreamURL.String != "" {
			eventURL = match.StreamURL.String
		}
		fmt.Fprintf(&ics, "URL:%s\r\n", eventURL)

		// Build location: League - Series (only include dash if both are non-empty)
		location := ""
		switch {
		case match.LeagueName != "" && match.SeriesName != "":
			location = fmt.Sprintf("%s - %s", match.LeagueName, match.SeriesName)
		case match.LeagueName != "":
			location = match.LeagueName
		case match.SeriesName != "":
			location = match.SeriesName
		}
		if location != "" {
			fmt.Fprintf(&ics, "LOCATION:%s\r\n", escapeICS(location))
		}

		fmt.Fprintf(&ics, "STATUS:%s\r\n", icsStatus(state))
		ics.WriteString("END:VEVENT\r\n")
	}

	ics.WriteString("END:VCALENDAR\r\n")
	return foldICS(ics.String())
}

// foldICS folds content lines longer than 75 octets as required by RFC 5545 section 3.1,
// without splitting a multi-byte UTF-8 character.
func foldICS(s string) string {
	const maxOctets = 75
	lines := strings.Split(strings.TrimSuffix(s, "\r\n"), "\r\n")
	var out strings.Builder
	for _, line := range lines {
		lineLen := 0
		for _, r := range line {
			size := utf8.RuneLen(r)
			if lineLen+size > maxOctets {
				out.WriteString("\r\n ")
				lineLen = 1
			}
			out.WriteRune(r)
			lineLen += size
		}
		out.WriteString("\r\n")
	}
	return out.String()
}

func escapeICS(s string) string {
	// Escape special characters for iCalendar format
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

const (
	// expiredNoticeDays is how many consecutive days the expired-link notice
	// repeats for, starting the day the hash was first seen missing.
	expiredNoticeDays = 30
	expiredNoticeName = "Esports Calendar (link expired)"
	expiredNoticeText = "Esports Calendar: this calendar link has expired"
	expiredNoticeBody = "This calendar subscription no longer works: its saved selections were lost " +
		"and could not be restored.\n\n" +
		"To keep getting matches, create a new calendar at %s/calendar, add the new link to " +
		"your calendar app, then remove this subscription.\n\n" +
		"Tip: if you pick exactly the same games, tiers, teams and options as before, you may get " +
		"the same link back, and this subscription will start working again on its own."
)

// generateExpiredICS builds the feed served for a calendar hash that has no
// stored selections. It holds one all-day notice repeating daily from since,
// so it shows on "today" in any calendar app. The UID and dates depend only on
// hash and since, so repeated polls update the event instead of duplicating it.
func generateExpiredICS(hash, baseURL string, since time.Time) string {
	start := since.UTC()
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	var ics strings.Builder
	ics.WriteString("BEGIN:VCALENDAR\r\n")
	ics.WriteString("VERSION:2.0\r\n")
	ics.WriteString("PRODID:-//EsportsCalendar//EN\r\n")
	ics.WriteString("CALSCALE:GREGORIAN\r\n")
	fmt.Fprintf(&ics, "X-WR-CALNAME:%s\r\n", escapeICS(expiredNoticeName))
	ics.WriteString("X-WR-TIMEZONE:UTC\r\n")
	ics.WriteString("BEGIN:VEVENT\r\n")
	fmt.Fprintf(&ics, "UID:expired-%s@%s\r\n", hash, baseURL)
	fmt.Fprintf(&ics, "DTSTAMP:%s\r\n", start.Format("20060102T150405Z"))
	fmt.Fprintf(&ics, "DTSTART;VALUE=DATE:%s\r\n", day.Format("20060102"))
	fmt.Fprintf(&ics, "DTEND;VALUE=DATE:%s\r\n", day.AddDate(0, 0, 1).Format("20060102"))
	fmt.Fprintf(&ics, "RRULE:FREQ=DAILY;COUNT=%d\r\n", expiredNoticeDays)
	fmt.Fprintf(&ics, "SUMMARY:%s\r\n", escapeICS(expiredNoticeText))
	fmt.Fprintf(&ics, "DESCRIPTION:%s\r\n", escapeICS(fmt.Sprintf(expiredNoticeBody, baseURL)))
	fmt.Fprintf(&ics, "URL:%s/calendar\r\n", baseURL)
	ics.WriteString("TRANSP:TRANSPARENT\r\n")
	ics.WriteString("END:VEVENT\r\n")
	ics.WriteString("END:VCALENDAR\r\n")
	return foldICS(ics.String())
}
