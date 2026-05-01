package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/feimaomiao/esportscalendar/components"
	"github.com/feimaomiao/esportscalendar/dbtypes"
)

const allGamesCacheKey = "all-games"

func (m *Middleware) IndexHandler(c *gin.Context) {
	m.Logger.Info("IndexHandler", zap.String("method", c.Request.Method), zap.String("path", c.Request.URL.Path))

	var games []dbtypes.Game
	var cacheHit bool

	// Check cache first
	cacheKey := allGamesCacheKey
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

	if isHTMXRequest(c) {
		setHTMXTitle(c, "Select Games - EsportsCalendar")
		component := components.IndexInner(options)
		if err := component.Render(m.Context, c.Writer); err != nil {
			m.Logger.Error("Failed to render index inner", zap.Error(err))
			c.String(http.StatusInternalServerError, "Failed to render page")
		}
		return
	}

	component := components.Index(options)
	if err := component.Render(m.Context, c.Writer); err != nil {
		m.Logger.Error("Failed to render index", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

// isHTMXRequest reports whether the incoming request was issued by HTMX.
func isHTMXRequest(c *gin.Context) bool {
	return c.Request.Header.Get("Hx-Request") == "true"
}

// setHTMXTitle sets the HX-Trigger header so the client updates document.title.
func setHTMXTitle(c *gin.Context, title string) {
	payload, err := json.Marshal(map[string]any{
		"page:title": map[string]string{"title": title},
	})
	if err != nil {
		return
	}
	c.Header("HX-Trigger", string(payload))
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

// gameOptions returns the same Option list used by IndexHandler/SecondPageHandler:
// all games minus the ignored set, with logo paths resolved.
func (m *Middleware) gameOptions() ([]components.Option, error) {
	var games []dbtypes.Game
	cacheKey := allGamesCacheKey
	if m.RedisCache != nil {
		if cachedJSON, ok := m.RedisCache.GetData(cacheKey); ok {
			//nolint:musttag // dbtypes.Game has json tags defined
			if err := json.Unmarshal([]byte(cachedJSON), &games); err != nil {
				m.Logger.Warn("Failed to unmarshal cached games", zap.Error(err))
				games = nil
			}
		}
	}
	//nolint:nestif // Cache-then-DB pattern with optional re-cache, mirrors IndexHandler.
	if games == nil {
		var err error
		games, err = m.DBConn.GetAllGames(m.Context)
		if err != nil {
			return nil, err
		}
		if m.RedisCache != nil {
			//nolint:musttag // dbtypes.Game has json tags defined
			if gamesJSON, marshalErr := json.Marshal(games); marshalErr == nil {
				if cacheErr := m.RedisCache.SetData(cacheKey, string(gamesJSON)); cacheErr != nil {
					m.Logger.Warn("Failed to cache games", zap.Error(cacheErr))
				}
			}
		}
	}

	ignored := map[int]bool{20: true, 25: true, 27: true, 29: true, 30: true}
	var options []components.Option
	for _, game := range games {
		if ignored[int(game.ID)] {
			continue
		}
		logo := components.DefaultLogo()
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
	return options, nil
}

// scheduleDefaults builds the default selection set used to render the initial
// match list server-side: every non-ignored game with its tier-1 leagues.
// Mirrors what schedule.js + game-selection.js produce on a fresh client.
func (m *Middleware) scheduleDefaults(options []components.Option) ([]int32, []int32) {
	gameIDs := make([]int32, 0, len(options))
	var leagueIDs []int32
	for _, opt := range options {
		gameID64, err := strconv.ParseInt(opt.ID, 10, 32)
		if err != nil {
			continue
		}
		gameID := int32(gameID64)
		gameIDs = append(gameIDs, gameID)

		leagues, leagueErr := m.DBConn.GetLeaguesByGameID(m.Context, gameID)
		if leagueErr != nil {
			m.Logger.Warn("Schedule defaults: failed to load leagues",
				zap.Int32("game_id", gameID), zap.Error(leagueErr))
			continue
		}
		for _, l := range leagues {
			if isTier1League(l.MinTier) {
				leagueIDs = append(leagueIDs, l.ID)
			}
		}
	}
	return gameIDs, leagueIDs
}

func isTier1League(minTier any) bool {
	if minTier == nil {
		return false
	}
	switch t := minTier.(type) {
	case int32:
		return t == 1
	case int64:
		return t == 1
	}
	return false
}

func (m *Middleware) ScheduleHandler(c *gin.Context) {
	const scheduleHistory = 30
	const scheduleHorizon = 30
	const defaultMaxTier = 2
	const defaultHideScores = true

	m.Logger.Info("Handler",
		zap.String("handler", "ScheduleHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	options, err := m.gameOptions()
	if err != nil {
		m.Logger.Error("Failed to fetch games", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to fetch games")
		return
	}

	gameIDs, leagueIDs := m.scheduleDefaults(options)

	var matches []dbtypes.GetFutureMatchesBySelectionsRow
	nowIndex := -1
	if len(gameIDs) > 0 && len(leagueIDs) > 0 {
		pastMatches, pastErr := m.DBConn.GetPastMatchesBySelections(m.Context, dbtypes.GetPastMatchesBySelectionsParams{
			GameIds:    gameIDs,
			LeagueIds:  leagueIDs,
			TeamIds:    nil,
			MaxTier:    defaultMaxTier,
			LimitCount: scheduleHistory,
		})
		if pastErr != nil {
			m.Logger.Warn("Schedule defaults: past fetch failed", zap.Error(pastErr))
		}
		futureMatches, futureErr := m.DBConn.GetFutureMatchesBySelections(
			m.Context,
			dbtypes.GetFutureMatchesBySelectionsParams{
				GameIds:    gameIDs,
				LeagueIds:  leagueIDs,
				TeamIds:    nil,
				MaxTier:    defaultMaxTier,
				LimitCount: scheduleHorizon,
			},
		)
		if futureErr != nil {
			m.Logger.Warn("Schedule defaults: future fetch failed", zap.Error(futureErr))
		}
		matches = make([]dbtypes.GetFutureMatchesBySelectionsRow, 0, len(pastMatches)+len(futureMatches))
		for _, pm := range pastMatches {
			matches = append(matches, dbtypes.GetFutureMatchesBySelectionsRow(pm))
		}
		matches = append(matches, futureMatches...)
		if len(pastMatches) > 0 && len(futureMatches) > 0 {
			nowIndex = len(pastMatches)
		}
	}

	// The page bakes in match data tied to wall-clock time (// now divider,
	// upcoming list). Caching it would serve a stale "now" boundary on revisit.
	c.Header("Cache-Control", "no-store")
	if isHTMXRequest(c) {
		setHTMXTitle(c, "Schedule - EsportsCalendar")
		component := components.SchedulePageInner(options, matches, nowIndex, defaultHideScores)
		if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
			m.Logger.Error("Failed to render schedule inner", zap.Error(renderErr))
			c.String(http.StatusInternalServerError, "Failed to render page")
		}
		return
	}

	component := components.SchedulePage(options, matches, nowIndex, defaultHideScores)
	if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
		m.Logger.Error("Failed to render schedule page", zap.Error(renderErr))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

// ScheduleAPIHandler accepts the same selections payload as /preview and returns
// an HTML fragment with up to scheduleHistory past + scheduleHorizon future matches.
func (m *Middleware) ScheduleAPIHandler(c *gin.Context) {
	const scheduleHistory = 30
	const scheduleHorizon = 30

	m.Logger.Info("Handler",
		zap.String("handler", "ScheduleAPIHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	var requestBody map[string]any
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		m.Logger.Warn("Failed to parse schedule body", zap.Error(err))
		c.String(http.StatusBadRequest, "Invalid request body")
		return
	}

	hideScores := false
	if v, ok := requestBody["hideScores"].(bool); ok {
		hideScores = v
	}

	selections, _ := requestBody["selections"].(map[string]any)
	if selections == nil {
		selections = requestBody
	}

	gameIDs, leagueIDs, teamIDs, maxTier := parseSelections(selections, m.Logger)
	if err := validateSelections(gameIDs, leagueIDs, teamIDs); err != nil {
		m.Logger.Warn("Invalid schedule selections", zap.Error(err))
		// Empty list is fine — render the empty state component so the client
		// still gets HTML to swap in.
		component := components.ScheduleMatchList(nil, hideScores, -1)
		if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
			c.String(http.StatusInternalServerError, "Failed to render page")
		}
		return
	}

	pastMatches, err := m.DBConn.GetPastMatchesBySelections(m.Context, dbtypes.GetPastMatchesBySelectionsParams{
		GameIds:    gameIDs,
		LeagueIds:  leagueIDs,
		TeamIds:    teamIDs,
		MaxTier:    maxTier,
		LimitCount: scheduleHistory,
	})
	if err != nil {
		m.Logger.Error("Failed to fetch past matches", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to fetch matches")
		return
	}

	futureMatches, err := m.DBConn.GetFutureMatchesBySelections(m.Context, dbtypes.GetFutureMatchesBySelectionsParams{
		GameIds:    gameIDs,
		LeagueIds:  leagueIDs,
		TeamIds:    teamIDs,
		MaxTier:    maxTier,
		LimitCount: scheduleHorizon,
	})
	if err != nil {
		m.Logger.Error("Failed to fetch future matches", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to fetch matches")
		return
	}

	combined := make([]dbtypes.GetFutureMatchesBySelectionsRow, 0, len(pastMatches)+len(futureMatches))
	for _, pm := range pastMatches {
		combined = append(combined, dbtypes.GetFutureMatchesBySelectionsRow(pm))
	}
	combined = append(combined, futureMatches...)

	// nowIndex marks the boundary between past and future; -1 means no divider.
	nowIndex := -1
	if len(pastMatches) > 0 && len(futureMatches) > 0 {
		nowIndex = len(pastMatches)
	}

	c.Header("Cache-Control", "no-store")
	component := components.ScheduleMatchList(combined, hideScores, nowIndex)
	if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
		m.Logger.Error("Failed to render schedule list", zap.Error(renderErr))
		c.String(http.StatusInternalServerError, "Failed to render page")
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

	// GET with no options: either an HTMX-driven flow that lost state or a
	// browser reload of /lts. For HTMX we still bounce home; for a full GET
	// we render the rehydration shell so the browser's sessionStorage can
	// rebuild the POST payload and swap the real page in.
	if c.Request.Method == http.MethodGet && len(selectedOptionIDs) == 0 {
		if isHTMXRequest(c) {
			c.Header("HX-Redirect", "/")
			c.Status(http.StatusOK)
			return
		}
		component := components.RehydratePage(
			"selectedGameOptions",
			"/lts",
			"form-options",
			"Leagues & Teams - EsportsCalendar",
		)
		if err := component.Render(m.Context, c.Writer); err != nil {
			m.Logger.Error("Failed to render lts rehydrate", zap.Error(err))
			c.String(http.StatusInternalServerError, "Failed to render page")
		}
		return
	}

	// Fetch all games (check cache first)
	var games []dbtypes.Game
	var cacheHit bool
	cacheKey := allGamesCacheKey
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
	if isHTMXRequest(c) {
		setHTMXTitle(c, "Leagues & Teams - EsportsCalendar")
		component := components.SecondPageInner(selectedOptions)
		if err := component.Render(m.Context, c.Writer); err != nil {
			m.Logger.Error("Failed to render second page inner", zap.Error(err))
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

// PreviewRehydrateHandler serves GET /preview with a small "restoring
// session" shell. JS reads the prior selections payload from sessionStorage
// (key: preview-selections) and POSTs it to /preview. If sessionStorage is
// empty the client redirects to /.
func (m *Middleware) PreviewRehydrateHandler(c *gin.Context) {
	m.Logger.Info("Handler",
		zap.String("handler", "PreviewRehydrateHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	component := components.RehydratePage(
		"preview-selections",
		"/preview",
		"json",
		"Preview - EsportsCalendar",
	)
	if err := component.Render(m.Context, c.Writer); err != nil {
		m.Logger.Error("Failed to render preview rehydrate", zap.Error(err))
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
	if err := validateSelections(gameIDs, leagueIDs, teamIDs); err != nil {
		m.Logger.Warn("Invalid preview selections",
			zap.String("request_id", requestID),
			zap.Error(err))
		c.String(http.StatusBadRequest, err.Error())
		return
	}
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
	var component templ.Component
	if isHTMXRequest(c) {
		setHTMXTitle(c, "Preview - EsportsCalendar")
		component = components.PreviewInner(matches, showingPast, hideScores)
	} else {
		component = components.PreviewPage(matches, showingPast, hideScores)
	}
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
