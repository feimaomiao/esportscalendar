package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

func (m *Middleware) LeagueOptionsHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler",
		zap.String("handler", "LeagueOptionsHandler"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))
	// Extract game ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/league-options/")
	gameID, err := strconv.ParseInt(path, 10, 32)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{
			"error":   true,
			"message": "Invalid game ID",
			"leagues": []any{},
		}
		writeJSON(w, response, m.Logger)
		return
	}

	// Check cache first
	cacheKey := fmt.Sprintf("league-options:%d", gameID)
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if jsonBytes, cacheOk := cachedData.([]byte); cacheOk {
			m.Logger.Info("Cache HIT",
				zap.String("handler", "LeagueOptionsHandler"),
				zap.String("cache_key", cacheKey),
				zap.Int64("game_id", gameID),
				zap.Int("response_size_bytes", len(jsonBytes)))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "public, max-age=600")
			w.Header().Set("X-Cache", "HIT")
			if _, writeErr := w.Write(jsonBytes); writeErr != nil {
				m.Logger.Error("Failed to write cached response", zap.Error(writeErr))
			}
			return
		}
	}

	m.Logger.Info("Cache MISS",
		zap.String("handler", "LeagueOptionsHandler"),
		zap.String("cache_key", cacheKey),
		zap.Int64("game_id", gameID))

	// Fetch leagues from database
	leagues, err := m.DBConn.GetLeaguesByGameID(m.Context, int32(gameID))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		var message string
		// Check if it's a connection error or other database issue
		if strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "connect") {
			message = "Database connection error. Please try again later."
		} else {
			message = "Unable to load leagues. Please refresh the page: " + err.Error()
		}
		response := map[string]any{
			"error":   true,
			"message": message,
			"leagues": []any{},
		}
		writeJSON(w, response, m.Logger)
		return
	}

	// Convert to response format
	type LeagueResponse struct {
		ID      int32  `json:"id"`
		Name    string `json:"name"`
		Image   string `json:"image"`
		IsTier1 bool   `json:"is_tier1"`
	}

	var leagueList []LeagueResponse
	for _, league := range leagues {
		image := "/static/images/default-logo.png"
		if league.ImageLink.Valid && league.ImageLink.String != "" {
			image = league.ImageLink.String
		}

		// Check if this is a tier 1 league
		isTier1 := false
		if league.MinTier != nil {
			if tier, ok := league.MinTier.(int32); ok && tier == 1 {
				isTier1 = true
			} else if tier64, ok64 := league.MinTier.(int64); ok64 && tier64 == 1 {
				isTier1 = true
			}
		}

		leagueList = append(leagueList, LeagueResponse{
			ID:      league.ID,
			Name:    league.Name,
			Image:   image,
			IsTier1: isTier1,
		})
	}

	// Build JSON response
	response := map[string]any{
		"error":   false,
		"message": "",
		"leagues": leagueList,
	}

	// Marshal to JSON bytes
	responseBytes, err := json.Marshal(response)
	if err != nil {
		m.Logger.Error("Failed to marshal response", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Cache the response
	m.Cache.Add(cacheKey, responseBytes)
	m.Logger.Info("Data cached",
		zap.String("handler", "LeagueOptionsHandler"),
		zap.String("cache_key", cacheKey),
		zap.Int("num_leagues", len(leagueList)),
		zap.Int("response_size_bytes", len(responseBytes)))

	// Return JSON response with HTTP cache headers (cache for 10 minutes)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.Header().Set("X-Cache", "MISS")
	if _, writeErr := w.Write(responseBytes); writeErr != nil {
		m.Logger.Error("Failed to write response", zap.Error(writeErr))
	}
}

func (m *Middleware) TeamOptionsHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler",
		zap.String("handler", "TeamOptionsHandler"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))
	// Extract game ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/team-options/")
	gameID, err := strconv.ParseInt(path, 10, 32)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{
			"error":   true,
			"message": "Invalid game ID",
			"teams":   []any{},
		}
		writeJSON(w, response, m.Logger)
		return
	}

	// Check cache first
	cacheKey := fmt.Sprintf("team-options:%d", gameID)
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if jsonBytes, cacheOk := cachedData.([]byte); cacheOk {
			m.Logger.Info("Cache HIT",
				zap.String("handler", "TeamOptionsHandler"),
				zap.String("cache_key", cacheKey),
				zap.Int64("game_id", gameID),
				zap.Int("response_size_bytes", len(jsonBytes)))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "public, max-age=600")
			w.Header().Set("X-Cache", "HIT")
			if _, writeErr := w.Write(jsonBytes); writeErr != nil {
				m.Logger.Error("Failed to write cached response", zap.Error(writeErr))
			}
			return
		}
	}

	m.Logger.Info("Cache MISS",
		zap.String("handler", "TeamOptionsHandler"),
		zap.String("cache_key", cacheKey),
		zap.Int64("game_id", gameID))

	// Fetch teams from database
	teams, err := m.DBConn.GetTeamsByGameID(m.Context, int32(gameID))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		var message string
		// Check if it's a connection error or other database issue
		if strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "connect") {
			message = "Database connection error. Please try again later."
		} else {
			message = "Unable to load teams. Please refresh the page: " + err.Error()
		}
		response := map[string]any{
			"error":   true,
			"message": message,
			"teams":   []any{},
		}
		writeJSON(w, response, m.Logger)
		return
	}

	// Convert to response format
	type TeamResponse struct {
		ID      int32  `json:"id"`
		Name    string `json:"name"`
		Acronym string `json:"acronym"`
		Image   string `json:"image"`
	}

	var teamList []TeamResponse
	for _, team := range teams {
		image := "/static/images/default-logo.png"
		if team.ImageLink.Valid && team.ImageLink.String != "" {
			image = team.ImageLink.String
		}

		acronym := ""
		if team.Acronym.Valid {
			acronym = team.Acronym.String
		}

		teamList = append(teamList, TeamResponse{
			ID:      team.ID,
			Name:    team.Name,
			Acronym: acronym,
			Image:   image,
		})
	}

	// Build JSON response
	response := map[string]any{
		"error":   false,
		"message": "",
		"teams":   teamList,
	}

	// Marshal to JSON bytes
	responseBytes, err := json.Marshal(response)
	if err != nil {
		m.Logger.Error("Failed to marshal response", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Cache the response
	m.Cache.Add(cacheKey, responseBytes)
	m.Logger.Info("Data cached",
		zap.String("handler", "TeamOptionsHandler"),
		zap.String("cache_key", cacheKey),
		zap.Int("num_teams", len(teamList)),
		zap.Int("response_size_bytes", len(responseBytes)))

	// Return JSON response with HTTP cache headers (cache for 10 minutes)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.Header().Set("X-Cache", "MISS")
	if _, writeErr := w.Write(responseBytes); writeErr != nil {
		m.Logger.Error("Failed to write response", zap.Error(writeErr))
	}
}
