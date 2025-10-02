package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/feimaomiao/esportscalendar/components"
	"github.com/feimaomiao/esportscalendar/dbtypes"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Middleware struct {
	DB      *pgxpool.Pool
	DBConn  *dbtypes.Queries
	Context context.Context
	Cache   *lru.Cache[string, any]
}

func InitMiddleHandler() Middleware {
	log.Println("[ENTRY] InitMiddleHandler() - Initializing middleware with database connection")
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
	}
}
func (m *Middleware) IndexHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ENTRY] IndexHandler() - Processing request from %s %s", r.Method, r.URL.Path)

	var games []dbtypes.Game

	// Check cache first
	cacheKey := "all-games"
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if cachedGames, ok := cachedData.([]dbtypes.Game); ok {
			log.Printf("[CACHE HIT] Serving cached games list")
			games = cachedGames
		}
	}

	// If not in cache, fetch from database
	if games == nil {
		log.Printf("[CACHE MISS] Fetching games from database")
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
	log.Printf("[ENTRY] SecondPageHandler() - Processing request from %s %s", r.Method, r.URL.Path)

	var selectedOptionIDs []string

	// Try to parse JSON body first
	if r.Header.Get("Content-Type") == "application/json" {
		var requestBody struct {
			Options []string `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err == nil {
			selectedOptionIDs = requestBody.Options
			log.Printf("[DEBUG] Parsed options from JSON body: %v", selectedOptionIDs)
		} else {
			log.Printf("[ERROR] Failed to parse JSON body: %v", err)
		}
	}

	// Fallback to query params or form data if no JSON body
	if len(selectedOptionIDs) == 0 {
		r.ParseForm()
		selectedOptionIDs = r.Form["options"]
		log.Printf("[DEBUG] Parsed options from form/query: %v", selectedOptionIDs)
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
	log.Printf("[ENTRY] LeagueOptionsHandler() - Processing API request from %s %s", r.Method, r.URL.Path)
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
			log.Printf("[CACHE HIT] Serving cached data for game ID: %d", gameID)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(jsonBytes)
			return
		}
	}

	log.Printf("[CACHE MISS] Fetching from database for game ID: %d", gameID)

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
		log.Printf("[ERROR] Failed to marshal response: %v", err)
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
	log.Printf("[ENTRY] TeamOptionsHandler() - Processing API request from %s %s", r.Method, r.URL.Path)
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
			log.Printf("[CACHE HIT] Serving cached data for game ID: %d", gameID)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(jsonBytes)
			return
		}
	}

	log.Printf("[CACHE MISS] Fetching teams from database for game ID: %d", gameID)

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
		log.Printf("[ERROR] Failed to marshal response: %v", err)
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
	log.Printf("[ENTRY] PreviewHandler() - Processing request from %s %s", r.Method, r.URL.Path)

	// Parse JSON body with selections
	var requestBody map[string]interface{}
	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			log.Printf("[ERROR] Failed to parse JSON body: %v", err)
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		log.Printf("[DEBUG] Received selections: %+v", requestBody)
	}

	// Extract game IDs, league IDs, and team IDs from selections
	var gameIDs []int32
	var leagueIDs []int32
	var teamIDs []int32

	for gameIDStr, selectionData := range requestBody {
		gameID, err := strconv.ParseInt(gameIDStr, 10, 32)
		if err != nil {
			log.Printf("[WARN] Invalid game ID: %s", gameIDStr)
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
		}
	}

	log.Printf("[DEBUG] Parsed IDs - Games: %v, Leagues: %v, Teams: %v", gameIDs, leagueIDs, teamIDs)

	// Fetch matches from database
	var matches []dbtypes.GetMatchesBySelectionsRow
	if len(gameIDs) > 0 {
		var err error
		matches, err = m.DBConn.GetMatchesBySelections(m.Context, dbtypes.GetMatchesBySelectionsParams{
			GameIds:   gameIDs,
			LeagueIds: leagueIDs,
			TeamIds:   teamIDs,
		})
		if err != nil {
			log.Printf("[ERROR] Failed to fetch matches: %v", err)
			http.Error(w, "Failed to fetch matches", http.StatusInternalServerError)
			return
		}
		log.Printf("[DEBUG] Found %d matches", len(matches))
	}

	// Render the preview page with matches
	component := components.PreviewPage(matches)
	component.Render(m.Context, w)
}
