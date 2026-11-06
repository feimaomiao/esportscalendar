package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

func (m *Middleware) ExportHandler(c *gin.Context) {
	m.Logger.Info("Handler",
		zap.String("handler", "ExportHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	// Parse JSON body with selections and hideScores
	var requestBody map[string]any
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		m.Logger.Error("Failed to parse JSON body", zap.Error(err))
		c.JSON(http.StatusBadRequest, map[string]string{jsonError: "Invalid request body"})
		return
	}
	m.Logger.Debug("Received request body for export", zap.Any("request_body", requestBody))

	// Validate selections before storing — refuse oversize / empty payloads.
	selections := unwrapSelections(requestBody)
	gIDs, lIDs, tIDs, _ := parseSelections(selections, m.Logger)
	if vErr := validateSelections(gIDs, lIDs, tIDs); vErr != nil {
		m.Logger.Warn("Invalid export selections", zap.Error(vErr))
		c.JSON(http.StatusBadRequest, map[string]string{jsonError: vErr.Error()})
		return
	}

	// Canonical JSON (sorted keys, default title format dropped) so the same
	// selections always hash to the same URL.
	jsonBytes, err := CanonicalExportJSON(requestBody)
	if err != nil {
		m.Logger.Error("Failed to marshal selections", zap.Error(err))
		c.JSON(http.StatusInternalServerError, map[string]string{jsonError: "Failed to process selections"})
		return
	}

	// Generate hash
	hash := CalendarHash(jsonBytes)
	previewLen := 100
	if len(jsonBytes) < previewLen {
		previewLen = len(jsonBytes)
	}
	m.Logger.Info("Exporting calendar",
		zap.String("hash", hash),
		zap.Int("payload_size", len(jsonBytes)),
		zap.String("payload_preview", string(jsonBytes[:previewLen])))

	// Store in database
	err = m.DBConn.InsertURLMapping(m.Context, dbtypes.InsertURLMappingParams{
		HashedKey: hash,
		ValueList: jsonBytes,
	})
	if err != nil {
		m.Logger.Error("Failed to store URL mapping", zap.Error(err), zap.String("hash", hash))
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create calendar link"})
		return
	}

	// Return the hash as JSON
	response := map[string]string{
		"hash": hash,
		"url":  fmt.Sprintf("%s/%s.ics", m.BaseURL, hash),
	}
	c.JSON(http.StatusOK, response)
}

func (m *Middleware) CalendarICSHandler(c *gin.Context) {
	m.Logger.Info("Handler",
		zap.String("handler", "CalendarICSHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	// Extract hash from URL path (format: /:hash.ics)
	path := strings.TrimPrefix(c.Request.URL.Path, "/")
	hash := strings.TrimSuffix(path, ".ics")

	if hash == "" || hash == path {
		c.String(http.StatusBadRequest, "Invalid calendar URL")
		return
	}

	m.Logger.Info("Looking up calendar",
		zap.String("hash", hash),
		zap.String("full_path", c.Request.URL.Path))

	// Retrieve selections from database
	mapping, err := m.DBConn.GetURLMapping(m.Context, hash)
	if errors.Is(err, pgx.ErrNoRows) && isCalendarHash(hash) {
		m.serveExpiredCalendar(c, hash)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		c.String(http.StatusNotFound, "Calendar not found")
		return
	}
	if err != nil {
		// Not a 404 or the notice feed: a transient DB failure must not tell
		// calendar apps the subscription is gone.
		m.Logger.Error("Failed to look up calendar",
			zap.String("hash", hash),
			zap.Error(err))
		c.String(http.StatusServiceUnavailable, "Calendar temporarily unavailable")
		return
	}

	m.Logger.Info("Calendar found in database", zap.String("hash", hash))

	// HEAD only probes the calendar, so it neither counts as an access nor
	// honours a cache refresh.
	isHead := c.Request.Method == http.MethodHead

	// Update access count
	if !isHead {
		err = m.DBConn.UpdateURLMappingAccessCount(m.Context, hash)
		if err != nil {
			m.Logger.Warn("Failed to update access count", zap.Error(err), zap.String("hash", hash))
		}
	}

	// Check if refresh is requested via query parameter
	refresh := c.Query("refresh") == "1" && !isHead
	if refresh && m.RedisCache != nil {
		m.Logger.Info("Cache refresh requested", zap.String("hash", hash))
		if delErr := m.RedisCache.DeleteICS(hash); delErr != nil {
			m.Logger.Warn("Failed to delete cache entry", zap.Error(delErr), zap.String("hash", hash))
		}
	}

	// Try to get from cache first (unless refresh was requested)
	var icsContent string
	var cacheHit bool
	if m.RedisCache != nil && !refresh {
		icsContent, cacheHit = m.RedisCache.GetICS(hash)
		if cacheHit {
			m.Logger.Info("Cache HIT", zap.String("hash", hash))
			// Set headers for iCalendar file download
			c.Status(http.StatusOK)
			c.Header("Content-Type", "text/calendar; charset=utf-8")
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"esports-calendar-%s.ics\"", hash))
			c.Header("Cache-Control", "public, max-age=3600") // Cache for 1 hour
			c.Header("X-Cache", "HIT")
			if _, writeErr := c.Writer.Write([]byte(icsContent)); writeErr != nil {
				m.Logger.Error("Failed to write calendar content", zap.Error(writeErr))
				return
			}
			m.Logger.Debug("Served calendar from cache", zap.String("hash", hash))
			return
		}
		m.Logger.Info("Cache MISS", zap.String("hash", hash))
	}

	// Cache miss or expired - generate new content
	// Parse stored data from JSON
	var storedData map[string]any
	if unmarshalErr := json.Unmarshal(mapping.ValueList, &storedData); unmarshalErr != nil {
		m.Logger.Error("Failed to parse stored data", zap.Error(unmarshalErr))
		c.String(http.StatusInternalServerError, "Invalid calendar data")
		return
	}

	// Extract hideScores flag (default to false)
	hideScores := false
	if hideScoresVal, ok := storedData["hideScores"].(bool); ok {
		hideScores = hideScoresVal
	}

	// Extract title format (default to the original "[Game] Tournament - Match" layout)
	format := parseTitleFormat(storedData["titleFormat"])

	// Extract selections (handle both old and new format)
	selections := unwrapSelections(storedData)

	// Extract game IDs, league IDs, team IDs, and per-game max tiers from selections
	gameIDs, leagueIDs, teamIDs, maxTiers := parseSelections(selections, m.Logger)
	m.Logger.Debug("Parsed IDs from selections",
		zap.Any("game_ids", gameIDs),
		zap.Any("league_ids", leagueIDs),
		zap.Any("team_ids", teamIDs),
		zap.Int32s("max_tiers", maxTiers),
		zap.Bool("hide_scores", hideScores),
		zap.String("title_format", string(format)))

	// Fetch matches from database (all-time, capped at 1000 most recent per game, filtered by tier)
	var matches []dbtypes.GetCalendarMatchesBySelectionsRow
	if len(gameIDs) > 0 {
		matches, err = m.DBConn.GetCalendarMatchesBySelections(m.Context, dbtypes.GetCalendarMatchesBySelectionsParams{
			GameIds:   gameIDs,
			LeagueIds: leagueIDs,
			TeamIds:   teamIDs,
			MaxTiers:  maxTiers,
		})
		if err != nil {
			// 503 as for the mapping lookup above: a transient DB failure must
			// not read as a dead subscription.
			m.Logger.Error("Failed to fetch matches", zap.Error(err))
			c.String(http.StatusServiceUnavailable, "Failed to generate calendar")
			return
		}
	}

	// Generate iCalendar format with hideScores flag and title format
	icsContent = generateICS(matches, hideScores, format, m.BaseURL)

	// Store in cache
	if m.RedisCache != nil {
		if cacheErr := m.RedisCache.SetICS(hash, icsContent); cacheErr != nil {
			m.Logger.Warn("Failed to cache ICS file", zap.Error(cacheErr), zap.String("hash", hash))
		}
	}

	// Set headers for iCalendar file download
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/calendar; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"esports-calendar-%s.ics\"", hash))
	c.Header("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	c.Header("X-Cache", "MISS")
	if _, writeErr := c.Writer.Write([]byte(icsContent)); writeErr != nil {
		m.Logger.Error("Failed to write calendar content", zap.Error(writeErr))
		return
	}

	m.Logger.Debug("Served calendar", zap.Int("match_count", len(matches)), zap.String("hash", hash))
}

// calendarExpiredKey marks, on the gin context, a .ics request answered with
// the expired-link notice, so telemetry does not count it as a live calendar.
const calendarExpiredKey = "calendar_expired"

// maxMissingCalendars caps missing_calendars, which any request for a random
// hash could otherwise grow without limit. The 2026 outage produced 134 rows.
const maxMissingCalendars = 10000

// serveExpiredCalendar answers a well-formed calendar hash that has no stored
// selections. Calendar apps never show an error body, so instead of a 404 it
// serves a feed whose only event tells the user to create a new calendar. The
// hash is recorded so recover-calendars can try to rebuild its mapping.
func (m *Middleware) serveExpiredCalendar(c *gin.Context, hash string) {
	c.Set(calendarExpiredKey, true)
	since := time.Now()
	// HEAD only probes the link, and `since` only feeds the body it never gets.
	if c.Request.Method != http.MethodHead {
		firstSeen, err := m.DBConn.RecordMissingCalendar(m.Context, dbtypes.RecordMissingCalendarParams{
			HashedKey: hash,
			MaxRows:   maxMissingCalendars,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// The table is full. The notice doesn't depend on the row existing.
			m.Logger.Warn("Missing calendar table is full", zap.String("hash", hash))
		case err != nil:
			m.Logger.Warn("Failed to record missing calendar", zap.Error(err), zap.String("hash", hash))
		case firstSeen.Valid:
			since = firstSeen.Time
		}
	}
	m.Logger.Info("Serving expired calendar notice", zap.String("hash", hash))

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/calendar; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"esports-calendar-%s.ics\"", hash))
	// no-store so the real feed replaces the notice as soon as the mapping is
	// recovered, rather than after a CDN or client cache expires.
	c.Header("Cache-Control", "no-store")
	if _, writeErr := c.Writer.WriteString(generateExpiredICS(hash, m.BaseURL, since)); writeErr != nil {
		m.Logger.Error("Failed to write expired calendar notice", zap.Error(writeErr))
	}
}
