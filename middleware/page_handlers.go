package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/feimaomiao/esportscalendar/components"
	"github.com/feimaomiao/esportscalendar/dbtypes"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (m *Middleware) IndexHandler(c *gin.Context) {
	m.Logger.Info("IndexHandler", zap.String("method", c.Request.Method), zap.String("path", c.Request.URL.Path))

	var games []dbtypes.Game
	var cacheHit bool

	// Check cache first
	cacheKey := "all-games"
	if m.RedisCache != nil {
		if cachedJSON, ok := m.RedisCache.GetData(cacheKey); ok {
			//nolint:musttag // dbtypes.Game has json tags defined
			if err := json.Unmarshal([]byte(cachedJSON), &games); err == nil {
				m.Logger.Info("Cache HIT",
					zap.String("handler", "IndexHandler"),
					zap.String("cache_key", cacheKey),
					zap.Int("num_games", len(games)))
				cacheHit = true
			} else {
				m.Logger.Warn("Failed to unmarshal cached games", zap.Error(err))
			}
		}
	}

	// If not in cache, fetch from database
	//nolint:nestif // Nested structure is readable and necessary for cache-then-db pattern
	if games == nil {
		m.Logger.Info("Cache MISS",
			zap.String("handler", "IndexHandler"),
			zap.String("cache_key", cacheKey))
		var err error
		games, err = m.DBConn.GetAllGames(m.Context)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch games")
			return
		}
		// Cache the games list
		if m.RedisCache != nil {
			//nolint:musttag // dbtypes.Game has json tags defined
			if gamesJSON, marshalErr := json.Marshal(games); marshalErr == nil {
				if cacheErr := m.RedisCache.SetData(cacheKey, string(gamesJSON)); cacheErr != nil {
					m.Logger.Warn("Failed to cache games", zap.Error(cacheErr))
				} else {
					m.Logger.Info("Data cached",
						zap.String("handler", "IndexHandler"),
						zap.String("cache_key", cacheKey),
						zap.Int("num_games", len(games)))
				}
			}
		}
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
	c.Header("Cache-Control", "public, max-age=300")
	if cacheHit {
		c.Header("X-Cache", "HIT")
	} else {
		c.Header("X-Cache", "MISS")
	}

	component := components.Index(options)
	if err := component.Render(m.Context, c.Writer); err != nil {
		m.Logger.Error("Failed to render index", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

func (m *Middleware) HowToUseHandler(c *gin.Context) {
	m.Logger.Info("Handler",
		zap.String("handler", "HowToUseHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	component := components.HowToUsePage()
	if err := component.Render(m.Context, c.Writer); err != nil {
		m.Logger.Error("Failed to render how-to-use page", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

func (m *Middleware) AboutHandler(c *gin.Context) {
	m.Logger.Info("Handler",
		zap.String("handler", "AboutHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	component := components.AboutPage()
	if err := component.Render(m.Context, c.Writer); err != nil {
		m.Logger.Error("Failed to render about page", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

func (m *Middleware) renderLoadingPage(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	if _, err := c.Writer.Write([]byte(`<!DOCTYPE html>
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

//nolint:gocognit // Handler complexity is acceptable for this use case
func (m *Middleware) SecondPageHandler(c *gin.Context) {
	m.Logger.Info("Handler",
		zap.String("handler", "SecondPageHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	var selectedOptionIDs []string

	// Parse form data first (HTMX default)
	if err := c.Request.ParseForm(); err == nil && len(c.Request.Form["options"]) > 0 {
		selectedOptionIDs = c.Request.Form["options"]
		m.Logger.Debug("Parsed options from form", zap.Strings("options", selectedOptionIDs))
	} else if c.Request.Header.Get("Content-Type") == "application/json" {
		// Fallback to JSON for backward compatibility
		var requestBody struct {
			Options []string `json:"options"`
		}
		if jsonErr := c.ShouldBindJSON(&requestBody); jsonErr == nil {
			selectedOptionIDs = requestBody.Options
			m.Logger.Debug("Parsed options from JSON body", zap.Strings("options", selectedOptionIDs))
		} else {
			m.Logger.Error("Failed to parse JSON body", zap.Error(jsonErr))
		}
	}

	// If GET request with no options, render a page that checks sessionStorage
	if c.Request.Method == http.MethodGet && len(selectedOptionIDs) == 0 {
		m.renderLoadingPage(c)
		return
	}

	// Fetch all games (check cache first)
	var games []dbtypes.Game
	var cacheHit bool
	cacheKey := "all-games"
	if m.RedisCache != nil {
		if cachedJSON, ok := m.RedisCache.GetData(cacheKey); ok {
			//nolint:musttag // dbtypes.Game has json tags defined
			if err := json.Unmarshal([]byte(cachedJSON), &games); err == nil {
				m.Logger.Info("Cache HIT",
					zap.String("handler", "SecondPageHandler"),
					zap.String("cache_key", cacheKey),
					zap.Int("num_games", len(games)))
				cacheHit = true
			} else {
				m.Logger.Warn("Failed to unmarshal cached games", zap.Error(err))
			}
		}
	}

	//nolint:nestif // Nested structure is readable and necessary for cache-then-db pattern
	if games == nil {
		m.Logger.Info("Cache MISS",
			zap.String("handler", "SecondPageHandler"),
			zap.String("cache_key", cacheKey))
		var err error
		games, err = m.DBConn.GetAllGames(m.Context)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch games")
			return
		}
		if m.RedisCache != nil {
			//nolint:musttag // dbtypes.Game has json tags defined
			if gamesJSON, marshalErr := json.Marshal(games); marshalErr == nil {
				if cacheErr := m.RedisCache.SetData(cacheKey, string(gamesJSON)); cacheErr != nil {
					m.Logger.Warn("Failed to cache games", zap.Error(cacheErr))
				} else {
					m.Logger.Info("Data cached",
						zap.String("handler", "SecondPageHandler"),
						zap.String("cache_key", cacheKey),
						zap.Int("num_games", len(games)))
				}
			}
		}
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
	c.Header("Cache-Control", "public, max-age=300")
	if cacheHit {
		c.Header("X-Cache", "HIT")
	} else {
		c.Header("X-Cache", "MISS")
	}

	// For HTMX partial updates
	if c.Request.Header.Get("Hx-Request") == "true" {
		component := components.SecondPageContent(selectedOptions)
		if err := component.Render(m.Context, c.Writer); err != nil {
			m.Logger.Error("Failed to render second page content", zap.Error(err))
			c.String(http.StatusInternalServerError, "Failed to render page")
		}
		return
	}

	// Full page load
	component := components.SecondPage(selectedOptions)
	if err := component.Render(m.Context, c.Writer); err != nil {
		m.Logger.Error("Failed to render second page", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

func (m *Middleware) PreviewHandler(c *gin.Context) {
	// Generate unique request ID for debugging
	requestID := c.GetHeader("X-Request-ID")
	if requestID == "" {
		requestID = c.RemoteIP() + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}

	m.Logger.Info("Handler",
		zap.String("handler", "PreviewHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.String("request_id", requestID))

	// Parse JSON body with selections and hideScores
	var requestBody map[string]any
	if c.Request.Header.Get("Content-Type") == "application/json" {
		if err := c.ShouldBindJSON(&requestBody); err != nil {
			m.Logger.Error("Failed to parse JSON body",
				zap.String("request_id", requestID),
				zap.Error(err))
			c.String(http.StatusBadRequest, "Invalid request body")
			return
		}
		m.Logger.Debug("Received request body",
			zap.String("request_id", requestID),
			zap.Any("request_body", requestBody))
	}

	// Extract hideScores flag (default to false)
	hideScores := false
	if hideScoresVal, ok := requestBody["hideScores"].(bool); ok {
		hideScores = hideScoresVal
	}

	// Extract selections (handle both old and new format)
	var selections map[string]any
	if selectionsVal, ok := requestBody["selections"].(map[string]any); ok {
		// New format with selections wrapper
		selections = selectionsVal
	} else {
		// Old format without wrapper
		selections = requestBody
	}

	// Extract game IDs, league IDs, team IDs, and max tier from selections
	gameIDs, leagueIDs, teamIDs, maxTier := parseSelections(selections, m.Logger)
	m.Logger.Info("Preview request parsed",
		zap.String("request_id", requestID),
		zap.Int("num_games", len(gameIDs)),
		zap.Int("num_leagues", len(leagueIDs)),
		zap.Int("num_teams", len(teamIDs)),
		zap.Int32("max_tier", maxTier),
		zap.Bool("hide_scores", hideScores),
		zap.Any("game_ids", gameIDs),
		zap.Any("league_ids", leagueIDs),
		zap.Any("team_ids", teamIDs))

	// Fetch matches from database - show up to 5 past and 5 future
	startTime := time.Now()
	matches, showingPast, err := m.fetchMatches(gameIDs, leagueIDs, teamIDs, maxTier)
	fetchDuration := time.Since(startTime)

	if err != nil {
		m.Logger.Error("Failed to fetch matches",
			zap.String("request_id", requestID),
			zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to fetch matches")
		return
	}

	m.Logger.Info("Preview matches fetched",
		zap.String("request_id", requestID),
		zap.Int("match_count", len(matches)),
		zap.Bool("showing_past", showingPast),
		zap.Duration("fetch_duration", fetchDuration))

	// Render the preview page with matches
	renderStart := time.Now()
	component := components.PreviewPage(matches, showingPast, hideScores)
	if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
		m.Logger.Error("Failed to render preview page",
			zap.String("request_id", requestID),
			zap.Error(renderErr))
		c.String(http.StatusInternalServerError, "Failed to render page")
		return
	}

	renderDuration := time.Since(renderStart)
	totalDuration := time.Since(startTime)

	m.Logger.Info("Preview request completed",
		zap.String("request_id", requestID),
		zap.Duration("render_duration", renderDuration),
		zap.Duration("total_duration", totalDuration))
}

// fetchMatches retrieves matches based on selections, showing up to 10 total matches.
// Prioritizes future matches and only uses past matches if there are no available future ones.
func (m *Middleware) fetchMatches(
	gameIDs, leagueIDs, teamIDs []int32,
	maxTier int32,
) ([]dbtypes.GetFutureMatchesBySelectionsRow, bool, error) {
	const totalLimit = 10
	var matches []dbtypes.GetFutureMatchesBySelectionsRow
	var showingPast bool

	if len(gameIDs) == 0 {
		return matches, showingPast, nil
	}

	// Fetch up to 10 future matches first (prioritize future matches)
	futureMatches, err := m.DBConn.GetFutureMatchesBySelections(m.Context, dbtypes.GetFutureMatchesBySelectionsParams{
		GameIds:    gameIDs,
		LeagueIds:  leagueIDs,
		TeamIds:    teamIDs,
		MaxTier:    maxTier,
		LimitCount: totalLimit,
	})
	if err != nil {
		return nil, false, err
	}

	m.Logger.Debug("Found future matches", zap.Int("count", len(futureMatches)))

	// Calculate how many past matches we need to fill up to 10 total
	remainingSlots := totalLimit - len(futureMatches)

	var pastMatches []dbtypes.GetPastMatchesBySelectionsRow
	if remainingSlots > 0 {
		// Only fetch past matches if we have remaining slots
		var pastErr error
		pastMatches, pastErr = m.DBConn.GetPastMatchesBySelections(m.Context, dbtypes.GetPastMatchesBySelectionsParams{
			GameIds:    gameIDs,
			LeagueIds:  leagueIDs,
			TeamIds:    teamIDs,
			MaxTier:    maxTier,
			LimitCount: int32(remainingSlots), // #nosec G115 -- remainingSlots is bounded by totalLimit (10)
		})
		if pastErr != nil {
			return nil, false, pastErr
		}

		m.Logger.Debug("Found past matches", zap.Int("count", len(pastMatches)))
	}

	// Convert past matches to the same type as future matches
	matches = make([]dbtypes.GetFutureMatchesBySelectionsRow, 0, len(pastMatches)+len(futureMatches))
	for _, pm := range pastMatches {
		matches = append(matches, dbtypes.GetFutureMatchesBySelectionsRow(pm))
	}

	// Combine: past matches (in ASC order) + future matches (in ASC order)
	matches = append(matches, futureMatches...)

	if len(futureMatches) == 0 && len(pastMatches) > 0 {
		showingPast = true
	}

	m.Logger.Debug("Final match count",
		zap.Int("past", len(pastMatches)),
		zap.Int("future", len(futureMatches)),
		zap.Int("total", len(matches)),
		zap.Bool("showing_past", showingPast))
	return matches, showingPast, nil
}
