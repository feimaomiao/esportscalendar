package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/feimaomiao/esportscalendar/components"
	"github.com/feimaomiao/esportscalendar/dbtypes"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Middleware struct {
	DB      *pgxpool.Pool
	DBConn  *dbtypes.Queries
	Context context.Context
	Cache   *lru.Cache[string, any]
	Logger  *zap.Logger
}

func InitMiddleHandler(logger *zap.Logger) Middleware {
	logger.Info("Initializing middleware with database connection")
	connStr := fmt.Sprintf("host=localhost port=5432 user=%s password=%s dbname=esports sslmode=disable",
		os.Getenv("postgres_user"),
		os.Getenv("postgres_password"))
	conn, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		panic(err)
	}

	dbConn := dbtypes.New(conn)

	// Initialize LRU cache with 32 entries
	// technically we should be able to cache the entire games, leagues and teams list?
	cache, err := lru.New[string, any](32)
	if err != nil {
		panic(err)
	}

	return Middleware{
		DB:      conn,
		DBConn:  dbConn,
		Context: context.Background(),
		Cache:   cache,
		Logger:  logger,
	}
}
func (m *Middleware) IndexHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("IndexHandler", zap.String("method", r.Method), zap.String("path", r.URL.Path))

	var games []dbtypes.Game

	// Check cache first
	cacheKey := "all-games"
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if cachedGames, ok := cachedData.([]dbtypes.Game); ok {
			m.Logger.Debug("Serving cached games list")
			games = cachedGames
		}
	}

	// If not in cache, fetch from database
	if games == nil {
		m.Logger.Debug("Fetching games from database")
		var err error
		games, err = m.DBConn.GetAllGames(m.Context)
		if err != nil {
			http.Error(w, "Failed to fetch games", http.StatusInternalServerError)
			return
		}
		// Cache the games list
		m.Cache.Add(cacheKey, games)
	}

	var options []components.Option
	for _, game := range games {
		logo := components.DefaultLogo()
		ignored := map[int]bool{20: true, 25: true, 27: true, 29: true, 30: true}

		if ignored[int(game.ID)] {
			continue
		}
		if game.Slug.Valid {
			logo = components.LogoPath(game.Slug.String) + ".png"
		}
		options = append(options, components.Option{
			ID:      fmt.Sprintf("%d", game.ID),
			Label:   game.Name,
			Logo:    logo,
			Checked: false,
		})
	}

	component := components.Index(options)
	component.Render(m.Context, w)
}

func (m *Middleware) SecondPageHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler", zap.String("handler", "SecondPageHandler"), zap.String("method", r.Method), zap.String("path", r.URL.Path))

	var selectedOptionIDs []string

	// Try to parse JSON body first
	if r.Header.Get("Content-Type") == "application/json" {
		var requestBody struct {
			Options []string `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err == nil {
			selectedOptionIDs = requestBody.Options
			m.Logger.Debug("Parsed options from JSON body", zap.Strings("options", selectedOptionIDs))
		} else {
			m.Logger.Error("Failed to parse JSON body", zap.Error(err))
		}
	}

	// Fallback to query params or form data if no JSON body
	if len(selectedOptionIDs) == 0 {
		r.ParseForm()
		selectedOptionIDs = r.Form["options"]
		m.Logger.Debug("Parsed options from form/query", zap.Strings("options", selectedOptionIDs))
	}

	// If GET request with no options, render a page that checks sessionStorage
	if r.Method == "GET" && len(selectedOptionIDs) == 0 {
		// Render a temporary page that will check sessionStorage and redirect
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<title>Loading...</title>
	<script>
		const savedGameOptions = sessionStorage.getItem('selectedGameOptions');
		if (savedGameOptions) {
			const gameIds = JSON.parse(savedGameOptions);
			fetch('/lts', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ options: gameIds })
			}).then(response => response.text())
			  .then(html => {
				document.open();
				document.write(html);
				document.close();
			  });
		} else {
			// No saved selections, redirect to home
			window.location.href = '/';
		}
	</script>
</head>
<body>
	<p>Loading...</p>
</body>
</html>`))
		return
	}

	// Fetch all games (check cache first)
	var games []dbtypes.Game
	cacheKey := "all-games"
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if cachedGames, ok := cachedData.([]dbtypes.Game); ok {
			games = cachedGames
		}
	}

	if games == nil {
		var err error
		games, err = m.DBConn.GetAllGames(m.Context)
		if err != nil {
			http.Error(w, "Failed to fetch games", http.StatusInternalServerError)
			return
		}
		m.Cache.Add(cacheKey, games)
	}

	// Build Option objects for selected games
	var selectedOptions []components.Option
	for _, selectedID := range selectedOptionIDs {
		for _, game := range games {
			if fmt.Sprintf("%d", game.ID) == selectedID {
				logo := components.DefaultLogo()
				if game.Slug.Valid {
					logo = components.LogoPath(game.Slug.String) + ".png"
				}
				selectedOptions = append(selectedOptions, components.Option{
					ID:    selectedID,
					Label: game.Name,
					Logo:  logo,
				})
				break
			}
		}
	}

	// For HTMX partial updates
	if r.Header.Get("HX-Request") == "true" {
		component := components.SecondPageContent(selectedOptions)
		component.Render(m.Context, w)
		return
	}

	// Full page load
	component := components.SecondPage(selectedOptions)
	component.Render(m.Context, w)
}
func (m *Middleware) LeagueOptionsHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler", zap.String("handler", "LeagueOptionsHandler"), zap.String("method", r.Method), zap.String("path", r.URL.Path))
	// Extract game ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/league-options/")
	gameID, err := strconv.ParseInt(path, 10, 32)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"error":   true,
			"message": "Invalid game ID",
			"leagues": []interface{}{},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Check cache first
	cacheKey := fmt.Sprintf("league-options:%d", gameID)
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if jsonBytes, ok := cachedData.([]byte); ok {
			m.Logger.Debug("Cache hit - serving cached data", zap.String("cache_key", cacheKey), zap.Int64("game_id", gameID))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(jsonBytes)
			return
		}
	}

	m.Logger.Debug("Cache miss - fetching from database", zap.String("cache_key", cacheKey), zap.Int64("game_id", gameID))

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
		response := map[string]interface{}{
			"error":   true,
			"message": message,
			"leagues": []interface{}{},
		}
		json.NewEncoder(w).Encode(response)
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
			} else if tier, ok := league.MinTier.(int64); ok && tier == 1 {
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
	response := map[string]interface{}{
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

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(responseBytes)
}

func (m *Middleware) TeamOptionsHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler", zap.String("handler", "TeamOptionsHandler"), zap.String("method", r.Method), zap.String("path", r.URL.Path))
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
		json.NewEncoder(w).Encode(response)
		return
	}

	// Check cache first
	cacheKey := fmt.Sprintf("team-options:%d", gameID)
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if jsonBytes, ok := cachedData.([]byte); ok {
			m.Logger.Debug("Cache hit - serving cached data", zap.String("cache_key", cacheKey), zap.Int64("game_id", gameID))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(jsonBytes)
			return
		}
	}

	m.Logger.Debug("Cache miss - fetching teams from database", zap.String("cache_key", cacheKey), zap.Int64("game_id", gameID))

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
		json.NewEncoder(w).Encode(response)
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

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(responseBytes)
}
func (m *Middleware) PreviewHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler", zap.String("handler", "PreviewHandler"), zap.String("method", r.Method), zap.String("path", r.URL.Path))

	// Parse JSON body with selections
	var requestBody map[string]interface{}
	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			m.Logger.Error("Failed to parse JSON body", zap.Error(err))
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		m.Logger.Debug("Received selections", zap.Any("selections", requestBody))
	}

	// Extract game IDs, league IDs, team IDs, and max tier from selections
	var gameIDs []int32
	var leagueIDs []int32
	var teamIDs []int32
	maxTier := int32(1) // Default to tier 1

	for gameIDStr, selectionData := range requestBody {
		gameID, err := strconv.ParseInt(gameIDStr, 10, 32)
		if err != nil {
			m.Logger.Warn("Invalid game ID", zap.String("game_id_str", gameIDStr), zap.Error(err))
			continue
		}
		gameIDs = append(gameIDs, int32(gameID))

		if selectionMap, ok := selectionData.(map[string]interface{}); ok {
			// Extract league IDs
			if leagues, ok := selectionMap["leagues"].([]interface{}); ok {
				for _, league := range leagues {
					if leagueID, ok := league.(float64); ok {
						leagueIDs = append(leagueIDs, int32(leagueID))
					}
				}
			}

			// Extract team IDs
			if teams, ok := selectionMap["teams"].([]interface{}); ok {
				for _, team := range teams {
					if teamID, ok := team.(float64); ok {
						teamIDs = append(teamIDs, int32(teamID))
					}
				}
			}

			// Extract max tier (use the minimum across all games to be most restrictive)
			if tierValue, ok := selectionMap["maxTier"].(float64); ok {
				tier := int32(tierValue)
				if tier < maxTier {
					maxTier = tier
				}
			}
		}
	}

	m.Logger.Debug("Parsed IDs from selections", zap.Any("game_ids", gameIDs), zap.Any("league_ids", leagueIDs), zap.Any("team_ids", teamIDs), zap.Int32("max_tier", maxTier))

	// Fetch matches from database - show minimum of 10 matches
	const minMatches = 10
	var matches []dbtypes.GetFutureMatchesBySelectionsRow
	var showingPast bool
	if len(gameIDs) > 0 {
		var err error
		// Try to get future matches first
		futureMatches, err := m.DBConn.GetFutureMatchesBySelections(m.Context, dbtypes.GetFutureMatchesBySelectionsParams{
			GameIds:   gameIDs,
			LeagueIds: leagueIDs,
			TeamIds:   teamIDs,
			MaxTier:   maxTier,
		})
		if err != nil {
			m.Logger.Error("Failed to fetch future matches", zap.Error(err))
			http.Error(w, "Failed to fetch matches", http.StatusInternalServerError)
			return
		}

		matches = futureMatches
		m.Logger.Debug("Found future matches", zap.Int("count", len(futureMatches)))

		// If we have fewer than 10 matches, backfill with past matches
		if len(matches) < minMatches {
			numNeeded := minMatches - len(matches)
			m.Logger.Debug("Need more matches - fetching past matches", zap.Int("num_needed", numNeeded), zap.Int("min_matches", minMatches))

			pastMatches, err := m.DBConn.GetPastMatchesBySelections(m.Context, dbtypes.GetPastMatchesBySelectionsParams{
				GameIds:   gameIDs,
				LeagueIds: leagueIDs,
				TeamIds:   teamIDs,
				MaxTier:   maxTier,
			})
			if err != nil {
				m.Logger.Error("Failed to fetch past matches", zap.Error(err))
				http.Error(w, "Failed to fetch matches", http.StatusInternalServerError)
				return
			}

			// Convert and prepend past matches (they're already in ASC order from SQL)
			// We need to prepend them since they come before future matches chronologically
			pastMatchesConverted := make([]dbtypes.GetFutureMatchesBySelectionsRow, 0, len(pastMatches))
			for _, pm := range pastMatches {
				pastMatchesConverted = append(pastMatchesConverted, dbtypes.GetFutureMatchesBySelectionsRow(pm))
				if len(pastMatchesConverted) >= minMatches-len(futureMatches) {
					break
				}
			}

			// Prepend past matches to future matches (both in ASC order, so chronological)
			matches = append(pastMatchesConverted, matches...)

			if len(futureMatches) == 0 {
				showingPast = true
			}
			m.Logger.Debug("Added past matches", zap.Int("past_matches_added", len(pastMatchesConverted)), zap.Int("total_matches", len(matches)))
		}

		m.Logger.Debug("Final match count", zap.Int("count", len(matches)), zap.Bool("showing_past", showingPast))
	}

	// Render the preview page with matches
	component := components.PreviewPage(matches, showingPast)
	component.Render(m.Context, w)
}

// generateHash creates a consistent hash from the selections JSON
func generateHash(data []byte) string {
	hash := sha256.Sum256(data)
	// Return first 16 characters for a shorter URL
	return hex.EncodeToString(hash[:])[:16]
}

func (m *Middleware) ExportHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler", zap.String("handler", "ExportHandler"), zap.String("method", r.Method), zap.String("path", r.URL.Path))

	// Parse JSON body with selections
	var requestBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		m.Logger.Error("Failed to parse JSON body", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}
	m.Logger.Debug("Received selections for export", zap.Any("selections", requestBody))

	// Generate canonical JSON (sorted keys for consistent hashing)
	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		m.Logger.Error("Failed to marshal selections", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to process selections"})
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
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create calendar link"})
		return
	}

	// Return the hash as JSON
	response := map[string]string{
		"hash": hash,
		"url":  fmt.Sprintf("http://localhost:8080//%s.ics", hash),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (m *Middleware) CalendarHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler", zap.String("handler", "CalendarHandler"), zap.String("method", r.Method), zap.String("path", r.URL.Path))

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

	// Parse selections from stored JSON
	var selections map[string]interface{}
	if err := json.Unmarshal(mapping.ValueList, &selections); err != nil {
		m.Logger.Error("Failed to parse stored selections", zap.Error(err))
		http.Error(w, "Invalid calendar data", http.StatusInternalServerError)
		return
	}

	// Extract game IDs, league IDs, team IDs, and max tier from selections
	var gameIDs []int32
	var leagueIDs []int32
	var teamIDs []int32
	maxTier := int32(1) // Default to tier 1

	for gameIDStr, selectionData := range selections {
		gameID, err := strconv.ParseInt(gameIDStr, 10, 32)
		if err != nil {
			m.Logger.Warn("Invalid game ID", zap.String("game_id_str", gameIDStr), zap.Error(err))
			continue
		}
		gameIDs = append(gameIDs, int32(gameID))

		if selectionMap, ok := selectionData.(map[string]interface{}); ok {
			// Extract league IDs
			if leagues, ok := selectionMap["leagues"].([]interface{}); ok {
				for _, league := range leagues {
					if leagueID, ok := league.(float64); ok {
						leagueIDs = append(leagueIDs, int32(leagueID))
					}
				}
			}

			// Extract team IDs
			if teams, ok := selectionMap["teams"].([]interface{}); ok {
				for _, team := range teams {
					if teamID, ok := team.(float64); ok {
						teamIDs = append(teamIDs, int32(teamID))
					}
				}
			}

			// Extract max tier (use the minimum across all games to be most restrictive)
			if tierValue, ok := selectionMap["maxTier"].(float64); ok {
				tier := int32(tierValue)
				if tier < maxTier {
					maxTier = tier
				}
			}
		}
	}

	m.Logger.Debug("Parsed IDs from selections", zap.Any("game_ids", gameIDs), zap.Any("league_ids", leagueIDs), zap.Any("team_ids", teamIDs), zap.Int32("max_tier", maxTier))

	// Fetch matches from database (7 days old and future, filtered by tier)
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
	icsContent := generateICS(matches)

	// Set headers for iCalendar file download
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"esports-calendar-%s.ics\"", hash))
	w.Write([]byte(icsContent))

	m.Logger.Debug("Served calendar", zap.Int("match_count", len(matches)), zap.String("hash", hash))
}

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
		// Assume 2 hour duration for matches
		endTime := startTime.Add(2 * 60 * 60 * 1000000000) // 2 hours in nanoseconds

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
		location := fmt.Sprintf("%s (%s)", match.TournamentName, match.GameName)
		ics.WriteString(fmt.Sprintf("LOCATION:%s\r\n", escapeICS(location)))

		// Mark finished matches differently
		if match.Finished {
			ics.WriteString("STATUS:CONFIRMED\r\n")
		} else {
			ics.WriteString("STATUS:CONFIRMED\r\n")
		}
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
