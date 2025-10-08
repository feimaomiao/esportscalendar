package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/feimaomiao/esportscalendar/dbtypes"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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
		c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	m.Logger.Debug("Received request body for export", zap.Any("request_body", requestBody))

	// Generate canonical JSON (sorted keys for consistent hashing)
	// This preserves both selections and hideScores in the stored data
	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		m.Logger.Error("Failed to marshal selections", zap.Error(err))
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to process selections"})
		return
	}

	// Generate hash
	hash := generateHash(jsonBytes)
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
		"url":  fmt.Sprintf("https://esportscalendar.app/%s.ics", hash),
	}
	c.JSON(http.StatusOK, response)
}

func (m *Middleware) CalendarHandler(c *gin.Context) {
	m.Logger.Info("Handler",
		zap.String("handler", "CalendarHandler"),
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
	if err != nil {
		m.Logger.Error("Calendar not found in database",
			zap.String("hash", hash),
			zap.String("url", c.Request.URL.Path),
			zap.Error(err))
		c.String(http.StatusNotFound, "Calendar not found")
		return
	}

	m.Logger.Info("Calendar found in database", zap.String("hash", hash))

	// Update access count
	err = m.DBConn.UpdateURLMappingAccessCount(m.Context, hash)
	if err != nil {
		m.Logger.Warn("Failed to update access count", zap.Error(err), zap.String("hash", hash))
	}

	// Try to get from cache first
	var icsContent string
	var cacheHit bool
	if m.RedisCache != nil {
		icsContent, cacheHit = m.RedisCache.GetICS(hash)
		if cacheHit {
			m.Logger.Info("Cache HIT", zap.String("hash", hash))
			// Set headers for iCalendar file download
			c.Status(http.StatusOK)
			c.Header("Content-Type", "text/calendar; charset=utf-8")
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"esports-calendar-%s.ics\"", hash))
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

	// Extract selections (handle both old and new format)
	var selections map[string]any
	if selectionsVal, ok := storedData["selections"].(map[string]any); ok {
		// New format with selections wrapper
		selections = selectionsVal
	} else {
		// Old format without wrapper
		selections = storedData
	}

	// Extract game IDs, league IDs, team IDs, and max tier from selections
	gameIDs, leagueIDs, teamIDs, maxTier := parseSelections(selections, m.Logger)
	m.Logger.Debug("Parsed IDs from selections",
		zap.Any("game_ids", gameIDs),
		zap.Any("league_ids", leagueIDs),
		zap.Any("team_ids", teamIDs),
		zap.Int32("max_tier", maxTier),
		zap.Bool("hide_scores", hideScores))

	// Fetch matches from database (14 days old and future, filtered by tier)
	var matches []dbtypes.GetCalendarMatchesBySelectionsRow
	if len(gameIDs) > 0 {
		matches, err = m.DBConn.GetCalendarMatchesBySelections(m.Context, dbtypes.GetCalendarMatchesBySelectionsParams{
			GameIds:   gameIDs,
			LeagueIds: leagueIDs,
			TeamIds:   teamIDs,
			MaxTier:   maxTier,
		})
		if err != nil {
			m.Logger.Error("Failed to fetch matches", zap.Error(err))
			c.String(http.StatusInternalServerError, "Failed to generate calendar")
			return
		}
	}

	// Generate iCalendar format with hideScores flag
	icsContent = generateICS(matches, hideScores)

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
	if _, writeErr := c.Writer.Write([]byte(icsContent)); writeErr != nil {
		m.Logger.Error("Failed to write calendar content", zap.Error(writeErr))
		return
	}

	m.Logger.Debug("Served calendar", zap.Int("match_count", len(matches)), zap.String("hash", hash))
}
