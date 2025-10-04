package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/feimaomiao/esportscalendar/dbtypes"
	"go.uber.org/zap"
)

func (m *Middleware) ExportHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler",
		zap.String("handler", "ExportHandler"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))

	// Parse JSON body with selections
	var requestBody map[string]any
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		m.Logger.Error("Failed to parse JSON body", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "Invalid request body"}, m.Logger)
		return
	}
	m.Logger.Debug("Received selections for export", zap.Any("selections", requestBody))

	// Generate canonical JSON (sorted keys for consistent hashing)
	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		m.Logger.Error("Failed to marshal selections", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "Failed to process selections"}, m.Logger)
		return
	}

	// Generate hash
	hash := generateHash(jsonBytes)
	m.Logger.Debug("Generated hash", zap.String("hash", hash))

	// Store in database
	err = m.DBConn.InsertURLMapping(m.Context, dbtypes.InsertURLMappingParams{
		HashedKey: hash,
		ValueList: jsonBytes,
	})
	if err != nil {
		m.Logger.Error("Failed to store URL mapping", zap.Error(err), zap.String("hash", hash))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "Failed to create calendar link"}, m.Logger)
		return
	}

	// Return the hash as JSON
	response := map[string]string{
		"hash": hash,
		"url":  fmt.Sprintf("http://localhost:8080//%s.ics", hash),
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, response, m.Logger)
}

func (m *Middleware) CalendarHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler",
		zap.String("handler", "CalendarHandler"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))

	// Extract hash from URL path (format: /:hash.ics)
	path := strings.TrimPrefix(r.URL.Path, "/")
	hash := strings.TrimSuffix(path, ".ics")

	if hash == "" || hash == path {
		http.Error(w, "Invalid calendar URL", http.StatusBadRequest)
		return
	}

	m.Logger.Debug("Looking up hash", zap.String("hash", hash))

	// Retrieve selections from database
	mapping, err := m.DBConn.GetURLMapping(m.Context, hash)
	if err != nil {
		m.Logger.Error("Hash not found", zap.String("hash", hash), zap.Error(err))
		http.Error(w, "Calendar not found", http.StatusNotFound)
		return
	}

	// Update access count
	err = m.DBConn.UpdateURLMappingAccessCount(m.Context, hash)
	if err != nil {
		m.Logger.Warn("Failed to update access count", zap.Error(err), zap.String("hash", hash))
	}

	// Try to get from cache first
	icsContent, cacheHit := m.ICSCache.Get(hash)
	if cacheHit {
		m.Logger.Info("Cache HIT", zap.String("hash", hash))
		// Set headers for iCalendar file download
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"esports-calendar-%s.ics\"", hash))
		if _, writeErr := w.Write([]byte(icsContent)); writeErr != nil {
			m.Logger.Error("Failed to write calendar content", zap.Error(writeErr))
			return
		}
		m.Logger.Debug("Served calendar from cache", zap.String("hash", hash))
		return
	}
	m.Logger.Info("Cache MISS", zap.String("hash", hash))

	// Cache miss or expired - generate new content
	// Parse selections from stored JSON
	var selections map[string]any
	if unmarshalErr := json.Unmarshal(mapping.ValueList, &selections); unmarshalErr != nil {
		m.Logger.Error("Failed to parse stored selections", zap.Error(unmarshalErr))
		http.Error(w, "Invalid calendar data", http.StatusInternalServerError)
		return
	}

	// Extract game IDs, league IDs, team IDs, and max tier from selections
	gameIDs, leagueIDs, teamIDs, maxTier := parseSelections(selections, m.Logger)
	m.Logger.Debug("Parsed IDs from selections",
		zap.Any("game_ids", gameIDs),
		zap.Any("league_ids", leagueIDs),
		zap.Any("team_ids", teamIDs),
		zap.Int32("max_tier", maxTier))

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
			http.Error(w, "Failed to generate calendar", http.StatusInternalServerError)
			return
		}
	}

	// Generate iCalendar format
	icsContent = generateICS(matches)

	// Store in cache
	if cacheErr := m.ICSCache.Set(hash, icsContent); cacheErr != nil {
		m.Logger.Warn("Failed to cache ICS file", zap.Error(cacheErr), zap.String("hash", hash))
	}

	// Set headers for iCalendar file download
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"esports-calendar-%s.ics\"", hash))
	if _, writeErr := w.Write([]byte(icsContent)); writeErr != nil {
		m.Logger.Error("Failed to write calendar content", zap.Error(writeErr))
		return
	}

	m.Logger.Debug("Served calendar", zap.Int("match_count", len(matches)), zap.String("hash", hash))
}
