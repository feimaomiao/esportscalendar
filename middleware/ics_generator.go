package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

func generateICS(matches []dbtypes.GetCalendarMatchesBySelectionsRow) string {
	var ics strings.Builder

	ics.WriteString("BEGIN:VCALENDAR\r\n")
	ics.WriteString("VERSION:2.0\r\n")
	ics.WriteString("PRODID:-//EsportsCalendar//EN\r\n")
	ics.WriteString("CALSCALE:GREGORIAN\r\n")
	ics.WriteString("X-WR-CALNAME:Esports Calendar\r\n")
	ics.WriteString("X-WR-TIMEZONE:UTC\r\n")

	for _, match := range matches {
		if !match.ExpectedStartTime.Valid {
			continue
		}

		startTime := match.ExpectedStartTime.Time
		// Calculate duration: 1 hour per game
		duration := time.Duration(match.AmountOfGames) * time.Hour
		endTime := startTime.Add(duration)

		ics.WriteString("BEGIN:VEVENT\r\n")
		ics.WriteString(fmt.Sprintf("UID:%d@localhost:8080/\r\n", match.ID))
		ics.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", startTime.UTC().Format("20060102T150405Z")))
		ics.WriteString(fmt.Sprintf("DTSTART:%s\r\n", startTime.UTC().Format("20060102T150405Z")))
		ics.WriteString(fmt.Sprintf("DTEND:%s\r\n", endTime.UTC().Format("20060102T150405Z")))

		// Include score in summary for finished matches
		summary := match.Name
		if match.Finished {
			summary = fmt.Sprintf("%s [%d-%d]", match.Name, match.Team1Score, match.Team2Score)
		}
		ics.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICS(summary)))

		// Build description with teams, league, tournament, and score for finished matches
		description := fmt.Sprintf("%s vs %s - %s - %s (%s)",
			match.Team1Name,
			match.Team2Name,
			match.TournamentName,
			match.LeagueName,
			match.GameName,
		)
		if match.Finished {
			description = fmt.Sprintf("%s vs %s [%d-%d] - %s - %s (%s)",
				match.Team1Name,
				match.Team2Name,
				match.Team1Score,
				match.Team2Score,
				match.TournamentName,
				match.LeagueName,
				match.GameName,
			)
		}
		ics.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICS(description)))

		// Build location: [Game] - [League] - [Series] - [Tournament], omitting empty fields
		var locationParts []string
		if match.GameName != "" {
			locationParts = append(locationParts, match.GameName)
		}
		if match.LeagueName != "" {
			locationParts = append(locationParts, match.LeagueName)
		}
		if match.SeriesName != "" {
			locationParts = append(locationParts, match.SeriesName)
		}
		if match.TournamentName != "" {
			locationParts = append(locationParts, match.TournamentName)
		}
		location := strings.Join(locationParts, " - ")
		ics.WriteString(fmt.Sprintf("LOCATION:%s\r\n", escapeICS(location)))

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
