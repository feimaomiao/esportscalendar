package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/feimaomiao/esportscalendar/components"
	"github.com/feimaomiao/esportscalendar/dbtypes"
	"go.uber.org/zap"
)

func (m *Middleware) IndexHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("IndexHandler", zap.String("method", r.Method), zap.String("path", r.URL.Path))

	var games []dbtypes.Game
	var cacheHit bool

	// Check cache first
	cacheKey := "all-games"
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if cachedGames, cacheOk := cachedData.([]dbtypes.Game); cacheOk {
			m.Logger.Info("Cache HIT",
				zap.String("handler", "IndexHandler"),
				zap.String("cache_key", cacheKey),
				zap.Int("num_games", len(cachedGames)))
			games = cachedGames
			cacheHit = true
		}
	}

	// If not in cache, fetch from database
	if games == nil {
		m.Logger.Info("Cache MISS",
			zap.String("handler", "IndexHandler"),
			zap.String("cache_key", cacheKey))
		var err error
		games, err = m.DBConn.GetAllGames(m.Context)
		if err != nil {
			http.Error(w, "Failed to fetch games", http.StatusInternalServerError)
			return
		}
		// Cache the games list
		m.Cache.Add(cacheKey, games)
		m.Logger.Info("Data cached",
			zap.String("handler", "IndexHandler"),
			zap.String("cache_key", cacheKey),
			zap.Int("num_games", len(games)))
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
			ID:      strconv.Itoa(int(game.ID)),
			Label:   game.Name,
			Logo:    logo,
			Checked: false,
		})
	}

	// Set HTTP cache headers (cache for 5 minutes)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if cacheHit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}

	component := components.Index(options)
	if err := component.Render(m.Context, w); err != nil {
		m.Logger.Error("Failed to render index", zap.Error(err))
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (m *Middleware) HowToUseHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler",
		zap.String("handler", "HowToUseHandler"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))

	component := components.HowToUsePage()
	if err := component.Render(m.Context, w); err != nil {
		m.Logger.Error("Failed to render how-to-use page", zap.Error(err))
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (m *Middleware) AboutHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler",
		zap.String("handler", "AboutHandler"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))

	component := components.AboutPage()
	if err := component.Render(m.Context, w); err != nil {
		m.Logger.Error("Failed to render about page", zap.Error(err))
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (m *Middleware) renderLoadingPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<title>Loading - EsportsCalendar</title>
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
				// Update title after document is rewritten
				document.title = 'Leagues & Teams - EsportsCalendar';
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
</html>`)); err != nil {
		m.Logger.Error("Failed to write response", zap.Error(err))
	}
}

func (m *Middleware) SecondPageHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler",
		zap.String("handler", "SecondPageHandler"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))

	var selectedOptionIDs []string

	// Parse form data first (HTMX default)
	if err := r.ParseForm(); err == nil && len(r.Form["options"]) > 0 {
		selectedOptionIDs = r.Form["options"]
		m.Logger.Debug("Parsed options from form", zap.Strings("options", selectedOptionIDs))
	} else if r.Header.Get("Content-Type") == "application/json" {
		// Fallback to JSON for backward compatibility
		var requestBody struct {
			Options []string `json:"options"`
		}
		if jsonErr := json.NewDecoder(r.Body).Decode(&requestBody); jsonErr == nil {
			selectedOptionIDs = requestBody.Options
			m.Logger.Debug("Parsed options from JSON body", zap.Strings("options", selectedOptionIDs))
		} else {
			m.Logger.Error("Failed to parse JSON body", zap.Error(jsonErr))
		}
	}

	// If GET request with no options, render a page that checks sessionStorage
	if r.Method == http.MethodGet && len(selectedOptionIDs) == 0 {
		m.renderLoadingPage(w)
		return
	}

	// Fetch all games (check cache first)
	var games []dbtypes.Game
	var cacheHit bool
	cacheKey := "all-games"
	if cachedData, ok := m.Cache.Get(cacheKey); ok {
		if cachedGames, cacheOk := cachedData.([]dbtypes.Game); cacheOk {
			m.Logger.Info("Cache HIT",
				zap.String("handler", "SecondPageHandler"),
				zap.String("cache_key", cacheKey),
				zap.Int("num_games", len(cachedGames)))
			games = cachedGames
			cacheHit = true
		}
	}

	if games == nil {
		m.Logger.Info("Cache MISS",
			zap.String("handler", "SecondPageHandler"),
			zap.String("cache_key", cacheKey))
		var err error
		games, err = m.DBConn.GetAllGames(m.Context)
		if err != nil {
			http.Error(w, "Failed to fetch games", http.StatusInternalServerError)
			return
		}
		m.Cache.Add(cacheKey, games)
		m.Logger.Info("Data cached",
			zap.String("handler", "SecondPageHandler"),
			zap.String("cache_key", cacheKey),
			zap.Int("num_games", len(games)))
	}

	// Build Option objects for selected games
	var selectedOptions []components.Option
	for _, selectedID := range selectedOptionIDs {
		for _, game := range games {
			if strconv.Itoa(int(game.ID)) == selectedID {
				logo := components.DefaultLogo()
				if game.Slug.Valid {
					logo = components.LogoPath(game.Slug.String) + ".png"
				}
				selectedOptions = append(selectedOptions, components.Option{
					ID:      selectedID,
					Label:   game.Name,
					Logo:    logo,
					Checked: false,
				})
				break
			}
		}
	}

	// Set HTTP cache headers (cache for 5 minutes)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if cacheHit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}

	// For HTMX partial updates
	if r.Header.Get("Hx-Request") == "true" {
		component := components.SecondPageContent(selectedOptions)
		if err := component.Render(m.Context, w); err != nil {
			m.Logger.Error("Failed to render second page content", zap.Error(err))
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	// Full page load
	component := components.SecondPage(selectedOptions)
	if err := component.Render(m.Context, w); err != nil {
		m.Logger.Error("Failed to render second page", zap.Error(err))
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (m *Middleware) PreviewHandler(w http.ResponseWriter, r *http.Request) {
	m.Logger.Info("Handler",
		zap.String("handler", "PreviewHandler"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))

	// Parse JSON body with selections
	var requestBody map[string]any
	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			m.Logger.Error("Failed to parse JSON body", zap.Error(err))
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		m.Logger.Debug("Received selections", zap.Any("selections", requestBody))
	}

	// Extract game IDs, league IDs, team IDs, and max tier from selections
	gameIDs, leagueIDs, teamIDs, maxTier := parseSelections(requestBody, m.Logger)
	m.Logger.Debug("Parsed IDs from selections",
		zap.Any("game_ids", gameIDs),
		zap.Any("league_ids", leagueIDs),
		zap.Any("team_ids", teamIDs),
		zap.Int32("max_tier", maxTier))

	// Fetch matches from database - show minimum of 10 matches
	matches, showingPast, err := m.fetchMatches(gameIDs, leagueIDs, teamIDs, maxTier)
	if err != nil {
		m.Logger.Error("Failed to fetch matches", zap.Error(err))
		http.Error(w, "Failed to fetch matches", http.StatusInternalServerError)
		return
	}

	// Render the preview page with matches
	component := components.PreviewPage(matches, showingPast)
	if renderErr := component.Render(m.Context, w); renderErr != nil {
		m.Logger.Error("Failed to render preview page", zap.Error(renderErr))
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// fetchMatches retrieves matches based on selections, backfilling with past matches if needed.
func (m *Middleware) fetchMatches(
	gameIDs, leagueIDs, teamIDs []int32,
	maxTier int32,
) ([]dbtypes.GetFutureMatchesBySelectionsRow, bool, error) {
	const minMatches = 10
	var matches []dbtypes.GetFutureMatchesBySelectionsRow
	var showingPast bool

	if len(gameIDs) == 0 {
		return matches, showingPast, nil
	}

	// Try to get future matches first
	futureMatches, err := m.DBConn.GetFutureMatchesBySelections(m.Context, dbtypes.GetFutureMatchesBySelectionsParams{
		GameIds:   gameIDs,
		LeagueIds: leagueIDs,
		TeamIds:   teamIDs,
		MaxTier:   maxTier,
	})
	if err != nil {
		return nil, false, err
	}

	matches = futureMatches
	m.Logger.Debug("Found future matches", zap.Int("count", len(futureMatches)))

	// If we have fewer than 10 matches, backfill with past matches
	if len(matches) < minMatches {
		numNeeded := minMatches - len(matches)
		m.Logger.Debug("Need more matches - fetching past matches",
			zap.Int("num_needed", numNeeded),
			zap.Int("min_matches", minMatches))

		pastMatches, pastErr := m.DBConn.GetPastMatchesBySelections(m.Context, dbtypes.GetPastMatchesBySelectionsParams{
			GameIds:   gameIDs,
			LeagueIds: leagueIDs,
			TeamIds:   teamIDs,
			MaxTier:   maxTier,
		})
		if pastErr != nil {
			return nil, false, pastErr
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
		m.Logger.Debug("Added past matches",
			zap.Int("past_matches_added", len(pastMatchesConverted)),
			zap.Int("total_matches", len(matches)))
	}

	m.Logger.Debug("Final match count", zap.Int("count", len(matches)), zap.Bool("showing_past", showingPast))
	return matches, showingPast, nil
}
