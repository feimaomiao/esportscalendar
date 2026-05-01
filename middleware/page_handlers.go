package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
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

// gameOptions returns the Option list used by Index, Fixtures, and Calendar
// handlers: all games minus the ignored set, with logo paths resolved.
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

// fixturesDefaults builds the default selection set used to render the initial
// match list server-side: every non-ignored game with its tier-1 leagues.
// Mirrors what fixtures.js + game-selection.js produce on a fresh client.
func (m *Middleware) fixturesDefaults(options []components.Option) ([]int32, []int32) {
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
			m.Logger.Warn("Fixtures defaults: failed to load leagues",
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

// Calendar bounds. The earliest navigable month is hard-coded — data prior to
// 2025-09 is incomplete or absent. The forward cap is a safety bound; bump
// calendarMaxMonthsAhead if you want users to page further into the future.
const (
	calendarMinYear        = 2025
	calendarMinMonth       = 9 // September
	calendarMaxMonthsAhead = 12
	calendarMonthMatchCap  = 500 // soft DB cap per month — way above any realistic month load
)

// monthAnchor returns the UTC midnight at the first day of the given month.
func monthAnchor(year int, month int) time.Time {
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
}

// addMonths returns (year, month) shifted by `delta` months, normalised so the
// month is always in [1, 12].
func addMonths(year int, month int, delta int) (int, int) {
	t := monthAnchor(year, month).AddDate(0, delta, 0)
	return t.Year(), int(t.Month())
}

// calendarMonthBounds returns the first day of the requested month (inclusive)
// and the first day of the following month (exclusive) — half-open range
// suitable for `expected_start_time >= start AND expected_start_time < end`.
func calendarMonthBounds(year int, month int) (time.Time, time.Time) {
	start := monthAnchor(year, month)
	end := start.AddDate(0, 1, 0)
	return start, end
}

// clampCalendarMonth pins (year, month) into the allowed window:
//   - lower bound: calendarMinYear-calendarMinMonth
//   - upper bound: today + calendarMaxMonthsAhead months
func clampCalendarMonth(year int, month int, now time.Time) (int, int) {
	target := monthAnchor(year, month)
	minBound := monthAnchor(calendarMinYear, calendarMinMonth)
	maxYear, maxMonth := addMonths(now.Year(), int(now.Month()), calendarMaxMonthsAhead)
	maxBound := monthAnchor(maxYear, maxMonth)
	if target.Before(minBound) {
		return calendarMinYear, calendarMinMonth
	}
	if target.After(maxBound) {
		return maxYear, maxMonth
	}
	return year, month
}

// defaultCalendarMonth returns the month to render on a fresh `/calendar`
// load: today's month when in range, otherwise the nearest in-range month.
func defaultCalendarMonth(now time.Time) (int, int) {
	return clampCalendarMonth(now.Year(), int(now.Month()), now)
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

func (m *Middleware) FixturesHandler(c *gin.Context) {
	const fixturesHistory = 30
	const fixturesHorizon = 30
	const defaultMaxTier = 2
	const defaultHideScores = true

	m.Logger.Info("Handler",
		zap.String("handler", "FixturesHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	options, err := m.gameOptions()
	if err != nil {
		m.Logger.Error("Failed to fetch games", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to fetch games")
		return
	}

	gameIDs, leagueIDs := m.fixturesDefaults(options)

	var matches []dbtypes.GetFutureMatchesBySelectionsRow
	nowIndex := -1
	if len(gameIDs) > 0 && len(leagueIDs) > 0 {
		pastMatches, pastErr := m.DBConn.GetPastMatchesBySelections(m.Context, dbtypes.GetPastMatchesBySelectionsParams{
			GameIds:    gameIDs,
			LeagueIds:  leagueIDs,
			TeamIds:    nil,
			MaxTier:    defaultMaxTier,
			LimitCount: fixturesHistory,
		})
		if pastErr != nil {
			m.Logger.Warn("Fixtures defaults: past fetch failed", zap.Error(pastErr))
		}
		futureMatches, futureErr := m.DBConn.GetFutureMatchesBySelections(
			m.Context,
			dbtypes.GetFutureMatchesBySelectionsParams{
				GameIds:    gameIDs,
				LeagueIds:  leagueIDs,
				TeamIds:    nil,
				MaxTier:    defaultMaxTier,
				LimitCount: fixturesHorizon,
			},
		)
		if futureErr != nil {
			m.Logger.Warn("Fixtures defaults: future fetch failed", zap.Error(futureErr))
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
		setHTMXTitle(c, "Fixtures - EsportsCalendar")
		component := components.FixturesPageInner(options, matches, nowIndex, defaultHideScores)
		if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
			m.Logger.Error("Failed to render fixtures inner", zap.Error(renderErr))
			c.String(http.StatusInternalServerError, "Failed to render page")
		}
		return
	}

	component := components.FixturesPage(options, matches, nowIndex, defaultHideScores)
	if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
		m.Logger.Error("Failed to render fixtures page", zap.Error(renderErr))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

// FixturesAPIHandler accepts the same selections payload as /api/calendar
// and returns an HTML fragment with up to fixturesHistory past + fixturesHorizon
// future matches.
func (m *Middleware) FixturesAPIHandler(c *gin.Context) {
	const fixturesHistory = 30
	const fixturesHorizon = 30

	m.Logger.Info("Handler",
		zap.String("handler", "FixturesAPIHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	var requestBody map[string]any
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		m.Logger.Warn("Failed to parse fixtures body", zap.Error(err))
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
		m.Logger.Warn("Invalid fixtures selections", zap.Error(err))
		// Empty list is fine — render the empty state component so the client
		// still gets HTML to swap in.
		component := components.FixturesMatchList(nil, hideScores, -1)
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
		LimitCount: fixturesHistory,
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
		LimitCount: fixturesHorizon,
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
	component := components.FixturesMatchList(combined, hideScores, nowIndex)
	if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
		m.Logger.Error("Failed to render fixtures list", zap.Error(renderErr))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

// calendarMatches loads matches for the requested [start, end) range using
// the supplied selections. Returns an empty slice when the request would
// produce a zero-result query (no games or no leagues/teams selected).
func (m *Middleware) calendarMatches(
	gameIDs, leagueIDs, teamIDs []int32,
	maxTier int32,
	start, end time.Time,
) ([]dbtypes.GetFutureMatchesBySelectionsRow, error) {
	if len(gameIDs) == 0 || (len(leagueIDs) == 0 && len(teamIDs) == 0) {
		return nil, nil
	}
	rows, err := m.DBConn.GetMatchesInRangeBySelections(m.Context, dbtypes.GetMatchesInRangeBySelectionsParams{
		StartTime: pgtype.Timestamp{ //nolint:exhaustruct // InfinityModifier zero value is Finite
			Time:  start,
			Valid: true,
		},
		EndTime: pgtype.Timestamp{ //nolint:exhaustruct // InfinityModifier zero value is Finite
			Time:  end,
			Valid: true,
		},
		GameIds:    gameIDs,
		TeamIds:    teamIDs,
		LeagueIds:  leagueIDs,
		MaxTier:    maxTier,
		LimitCount: calendarMonthMatchCap,
	})
	if err != nil {
		return nil, err
	}
	out := make([]dbtypes.GetFutureMatchesBySelectionsRow, len(rows))
	for i, r := range rows {
		out[i] = dbtypes.GetFutureMatchesBySelectionsRow(r)
	}
	return out, nil
}

// CalendarHandler renders the initial /calendar page. Default month is today
// (clamped into [calendarMin, today + calendarMaxMonthsAhead]). The handler
// preloads the same default selections as /fixtures so a cold visit shows
// matches without waiting on a client-side fetch.
func (m *Middleware) CalendarHandler(c *gin.Context) {
	const defaultHideScores = true

	m.Logger.Info("Handler",
		zap.String("handler", "CalendarHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	options, err := m.gameOptions()
	if err != nil {
		m.Logger.Error("Failed to fetch games", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to fetch games")
		return
	}

	now := time.Now().UTC()
	year, month := defaultCalendarMonth(now)
	maxYear, maxMonth := addMonths(now.Year(), int(now.Month()), calendarMaxMonthsAhead)

	gameIDs, leagueIDs := m.fixturesDefaults(options)
	start, end := calendarMonthBounds(year, month)
	matches, err := m.calendarMatches(gameIDs, leagueIDs, nil, defaultMaxTier, start, end)
	if err != nil {
		m.Logger.Warn("Calendar default month fetch failed", zap.Error(err))
		matches = nil
	}

	// Same reasoning as FixturesHandler: page bakes in wall-clock-relative
	// markup (today highlight), so we don't want shared HTTP cache.
	c.Header("Cache-Control", "no-store")

	if isHTMXRequest(c) {
		setHTMXTitle(c, "Calendar - EsportsCalendar")
		component := components.CalendarPageInner(
			options, year, month,
			calendarMinYear, calendarMinMonth,
			maxYear, maxMonth,
			matches, defaultHideScores,
		)
		if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
			m.Logger.Error("Failed to render calendar inner", zap.Error(renderErr))
			c.String(http.StatusInternalServerError, "Failed to render page")
		}
		return
	}

	component := components.CalendarPage(
		options, year, month,
		calendarMinYear, calendarMinMonth,
		maxYear, maxMonth,
		matches, defaultHideScores,
	)
	if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
		m.Logger.Error("Failed to render calendar page", zap.Error(renderErr))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}

// CalendarAPIHandler returns an HTML fragment for one month of the calendar.
// Request JSON: { selections: { ... }, year: 2026, month: 5, hideScores: bool }.
func (m *Middleware) CalendarAPIHandler(c *gin.Context) {
	m.Logger.Info("Handler",
		zap.String("handler", "CalendarAPIHandler"),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path))

	var requestBody map[string]any
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		m.Logger.Warn("Failed to parse calendar body", zap.Error(err))
		c.String(http.StatusBadRequest, "Invalid request body")
		return
	}

	hideScores := false
	if v, ok := requestBody["hideScores"].(bool); ok {
		hideScores = v
	}

	yearF, _ := requestBody["year"].(float64)
	monthF, _ := requestBody["month"].(float64)
	year := int(yearF)
	month := int(monthF)
	if year == 0 || month < 1 || month > 12 {
		c.String(http.StatusBadRequest, "Invalid year/month")
		return
	}
	year, month = clampCalendarMonth(year, month, time.Now().UTC())

	selections, _ := requestBody["selections"].(map[string]any)
	if selections == nil {
		selections = requestBody
	}
	gameIDs, leagueIDs, teamIDs, maxTier := parseSelections(selections, m.Logger)
	if err := validateSelections(gameIDs, leagueIDs, teamIDs); err != nil {
		// Render an empty grid so the client gets HTML to swap in.
		m.Logger.Warn("Invalid calendar selections", zap.Error(err))
		component := components.CalendarMonthGrid(year, month, nil, hideScores)
		if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
			c.String(http.StatusInternalServerError, "Failed to render page")
		}
		return
	}

	start, end := calendarMonthBounds(year, month)
	matches, err := m.calendarMatches(gameIDs, leagueIDs, teamIDs, maxTier, start, end)
	if err != nil {
		m.Logger.Error("Failed to fetch calendar matches", zap.Error(err))
		c.String(http.StatusInternalServerError, "Failed to fetch matches")
		return
	}

	c.Header("Cache-Control", "no-store")
	component := components.CalendarMonthGrid(year, month, matches, hideScores)
	if renderErr := component.Render(m.Context, c.Writer); renderErr != nil {
		m.Logger.Error("Failed to render calendar grid", zap.Error(renderErr))
		c.String(http.StatusInternalServerError, "Failed to render page")
	}
}
