package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

func generateICS(matches []dbtypes.GetCalendarMatchesBySelectionsRow, hideScores bool, baseURL string) string {
	var ics strings.Builder

	ics.WriteString("BEGIN:VCALENDAR\r\n")
	ics.WriteString("VERSION:2.0\r\n")
	ics.WriteString("PRODID:-//EsportsCalendar//EN\r\n")
	ics.WriteString("CALSCALE:GREGORIAN\r\n")
	ics.WriteString("X-WR-CALNAME:Esports Calendar\r\n")
	ics.WriteString("X-WR-TIMEZONE:UTC\r\n")

	for _, match := range matches {
		// Get team names with fallback to "TBD"
		team1Name := "TBD"
		if match.Team1Name.Valid {
			team1Name = match.Team1Name.String
		}
		team2Name := "TBD"
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
		fmt.Fprintf(&ics, "DTSTAMP:%s\r\n", startTime.UTC().Format("20060102T150405Z"))
		fmt.Fprintf(&ics, "DTSTART:%s\r\n", startTime.UTC().Format("20060102T150405Z"))
		fmt.Fprintf(&ics, "DTEND:%s\r\n", endTime.UTC().Format("20060102T150405Z"))
		fmt.Fprintf(&ics, "URL:%s\r\n", baseURL)

		// Build summary: [Game] Tournament - Match Name (omit tournament if empty)
		// Add scores to title if match is finished and hideScores is false
		summary := fmt.Sprintf("[%s] %s", match.GameName, match.Name)
		if match.TournamentName != "" {
			summary = fmt.Sprintf("[%s] %s - %s", match.GameName, match.TournamentName, match.Name)
		}
		if match.Finished && !hideScores {
			// Add score to the title
			summary = fmt.Sprintf("%s [%d-%d]", summary, match.Team1Score, match.Team2Score)
		}
		fmt.Fprintf(&ics, "SUMMARY:%s\r\n", escapeICS(summary))

		// Build description with teams, league, tournament, and score for finished matches
		description := fmt.Sprintf("%s vs %s - %s - %s (%s)",
			team1Name,
			team2Name,
			match.TournamentName,
			match.LeagueName,
			match.GameName,
		)
		if match.Finished {
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
		// Add stream URL to description if available
		if match.StreamURL.Valid && match.StreamURL.String != "" {
			description += "\n\nWatch live: " + match.StreamURL.String
		}
		fmt.Fprintf(&ics, "DESCRIPTION:%s\r\n", escapeICS(description))

		// Emit standard iCalendar URL property so calendar clients render a clickable link.
		if match.StreamURL.Valid && match.StreamURL.String != "" {
			fmt.Fprintf(&ics, "URL:%s\r\n", match.StreamURL.String)
		}

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

		// All matches are confirmed
		ics.WriteString("STATUS:CONFIRMED\r\n")
		ics.WriteString("END:VEVENT\r\n")
	}

	ics.WriteString("END:VCALENDAR\r\n")
	return ics.String()
}

func escapeICS(s string) string {
	// Escape special characters for iCalendar format
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
