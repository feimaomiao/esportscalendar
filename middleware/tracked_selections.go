package middleware

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

const (
	// A calendar is active if a calendar app fetched it within this window.
	activeCalendarDays = 7
	// Leagues and teams are reported for the most-followed N per game, which
	// bounds metric cardinality and keeps per-game dashboard filters useful.
	trackedTopPerGame    = 10
	trackedRefreshPeriod = 5 * time.Minute
	trackedQueryTimeout  = 10 * time.Second
)

// TrackedSource is the database surface the tracked-selection gauges read.
// Implemented in production by dbtypes.Querier.
type TrackedSource interface {
	GetActiveURLMappingSelections(ctx context.Context, days int32) ([][]byte, error)
	GetAllGames(ctx context.Context) ([]dbtypes.Game, error)
	GetLeagueNamesByIDs(ctx context.Context, ids []int32) ([]dbtypes.GetLeagueNamesByIDsRow, error)
	GetTeamNamesByIDs(ctx context.Context, ids []int32) ([]dbtypes.GetTeamNamesByIDsRow, error)
}

// gameItem identifies a league or team under the game it was selected for.
type gameItem struct {
	gameID int32
	id     int32
}

type gameTier struct {
	gameID int32
	tier   int32
}

// selectionCounts is how many active calendars include each selection.
type selectionCounts struct {
	calendars int64
	games     map[int32]int64
	leagues   map[gameItem]int64
	teams     map[gameItem]int64
	tiers     map[gameTier]int64
}

// countSelections tallies stored calendar payloads. Each calendar counts at
// most once per game, league and team, even if an ID is listed twice.
// Undecodable rows are skipped.
func countSelections(rows [][]byte) selectionCounts {
	counts := selectionCounts{
		calendars: 0,
		games:     map[int32]int64{},
		leagues:   map[gameItem]int64{},
		teams:     map[gameItem]int64{},
		tiers:     map[gameTier]int64{},
	}
	for _, row := range rows {
		var payload map[string]any
		if err := json.Unmarshal(row, &payload); err != nil {
			continue
		}
		counts.calendars++

		seenLeagues := map[gameItem]bool{}
		seenTeams := map[gameItem]bool{}
		for gameKey, data := range unwrapSelections(payload) {
			gameID, err := strconv.ParseInt(gameKey, 10, 32)
			if err != nil || gameID <= 0 {
				continue
			}
			gid := int32(gameID)
			leagues, teams, tier := parseGameSelection(data)
			counts.games[gid]++
			counts.tiers[gameTier{gameID: gid, tier: tier}]++
			for _, id := range leagues {
				seenLeagues[gameItem{gameID: gid, id: id}] = true
			}
			for _, id := range teams {
				seenTeams[gameItem{gameID: gid, id: id}] = true
			}
		}
		for k := range seenLeagues {
			counts.leagues[k]++
		}
		for k := range seenTeams {
			counts.teams[k]++
		}
	}
	return counts
}

type itemCount struct {
	item  gameItem
	count int64
}

// topPerGame returns the n most-followed items for each game, ordered by
// count descending then ID ascending so ties are deterministic.
func topPerGame(counts map[gameItem]int64, n int) []itemCount {
	all := make([]itemCount, 0, len(counts))
	for item, count := range counts {
		all = append(all, itemCount{item: item, count: count})
	}
	slices.SortFunc(all, func(a, b itemCount) int {
		return cmp.Or(
			cmp.Compare(a.item.gameID, b.item.gameID),
			cmp.Compare(b.count, a.count),
			cmp.Compare(a.item.id, b.item.id),
		)
	})
	out := make([]itemCount, 0, len(all))
	perGame := map[int32]int{}
	for _, ic := range all {
		if perGame[ic.item.gameID] < n {
			out = append(out, ic)
			perGame[ic.item.gameID]++
		}
	}
	return out
}

// tierLabel maps a tier ceiling to its UI name: 0 disables tier auto-include.
func tierLabel(tier int32) string {
	labels := []string{"off", "S", "A", "B", "C", "D"}
	if tier < 0 || int(tier) >= len(labels) {
		return strconv.Itoa(int(tier))
	}
	return labels[tier]
}

// trackedSnapshot is the labelled observation set for one refresh.
type trackedSnapshot struct {
	calendars int64
	games     []observation
	leagues   []observation
	teams     []observation
	tiers     []observation
}

type observation struct {
	value int64
	attrs metric.MeasurementOption
}

// loadTrackedSnapshot queries active calendars and resolves names for the
// reported games, leagues and teams. Missing names fall back to the ID.
func loadTrackedSnapshot(ctx context.Context, src TrackedSource) (*trackedSnapshot, error) {
	rows, err := src.GetActiveURLMappingSelections(ctx, activeCalendarDays)
	if err != nil {
		return nil, fmt.Errorf("failed to load active calendars: %w", err)
	}
	counts := countSelections(rows)
	topLeagues := topPerGame(counts.leagues, trackedTopPerGame)
	topTeams := topPerGame(counts.teams, trackedTopPerGame)

	games, err := src.GetAllGames(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load game names: %w", err)
	}
	gameNames := make(map[int32]string, len(games))
	for _, g := range games {
		gameNames[g.ID] = g.Name
	}
	gameName := func(id int32) string {
		if name, ok := gameNames[id]; ok {
			return name
		}
		return strconv.Itoa(int(id))
	}

	leagueRows, err := src.GetLeagueNamesByIDs(ctx, itemIDs(topLeagues))
	if err != nil {
		return nil, fmt.Errorf("failed to load league names: %w", err)
	}
	leagueNames := make(map[int32]string, len(leagueRows))
	for _, l := range leagueRows {
		leagueNames[l.ID] = l.Name
	}

	teamRows, err := src.GetTeamNamesByIDs(ctx, itemIDs(topTeams))
	if err != nil {
		return nil, fmt.Errorf("failed to load team names: %w", err)
	}
	teamNames := make(map[int32]string, len(teamRows))
	for _, t := range teamRows {
		teamNames[t.ID] = t.Name
	}

	snap := &trackedSnapshot{
		calendars: counts.calendars,
		games:     make([]observation, 0, len(counts.games)),
		leagues:   make([]observation, 0, len(topLeagues)),
		teams:     make([]observation, 0, len(topTeams)),
		tiers:     make([]observation, 0, len(counts.tiers)),
	}
	for id, count := range counts.games {
		snap.games = append(snap.games, observation{count, metric.WithAttributes(
			attribute.String("game", gameName(id)),
		)})
	}
	for gt, count := range counts.tiers {
		snap.tiers = append(snap.tiers, observation{count, metric.WithAttributes(
			attribute.String("game", gameName(gt.gameID)),
			attribute.String("tier", tierLabel(gt.tier)),
		)})
	}
	// IDs are included because names repeat across games (e.g. a team with
	// rosters in several titles), and identical attributes would merge series.
	for _, ic := range topLeagues {
		snap.leagues = append(snap.leagues, observation{ic.count, metric.WithAttributes(
			attribute.String("game", gameName(ic.item.gameID)),
			attribute.String("league", nameOrID(leagueNames, ic.item.id)),
			attribute.Int("league_id", int(ic.item.id)),
		)})
	}
	for _, ic := range topTeams {
		snap.teams = append(snap.teams, observation{ic.count, metric.WithAttributes(
			attribute.String("game", gameName(ic.item.gameID)),
			attribute.String("team", nameOrID(teamNames, ic.item.id)),
			attribute.Int("team_id", int(ic.item.id)),
		)})
	}
	return snap, nil
}

func itemIDs(items []itemCount) []int32 {
	ids := make([]int32, len(items))
	for i, ic := range items {
		ids[i] = ic.item.id
	}
	return ids
}

func nameOrID(names map[int32]string, id int32) string {
	if name, ok := names[id]; ok {
		return name
	}
	return strconv.Itoa(int(id))
}

// trackedCache refreshes the snapshot at most every trackedRefreshPeriod, so
// the 5 replicas don't each scan url_mappings on every metric export.
type trackedCache struct {
	mu        sync.Mutex
	src       TrackedSource
	snap      *trackedSnapshot
	fetchedAt time.Time
}

// get returns the cached snapshot, refreshing it when stale. A failed refresh
// returns the error and nothing else, so gauges skip the cycle instead of
// reporting stale or zero values; the next collection retries.
func (c *trackedCache) get(ctx context.Context) (*trackedSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snap != nil && time.Since(c.fetchedAt) < trackedRefreshPeriod {
		return c.snap, nil
	}
	ctx, cancel := context.WithTimeout(ctx, trackedQueryTimeout)
	defer cancel()
	snap, err := loadTrackedSnapshot(ctx, c.src)
	if err != nil {
		c.snap = nil
		return nil, err
	}
	c.snap, c.fetchedAt = snap, time.Now()
	return snap, nil
}

// registerTrackedGauges reports what active calendars follow: games, leagues
// (tournaments), teams and per-game tier ceilings. Every replica reports the
// same values, so dashboards should aggregate them with max(), not sum().
func registerTrackedGauges(meter metric.Meter, src TrackedSource, logger *zap.Logger) error {
	gauge := func(name, desc string) (metric.Int64ObservableGauge, error) {
		g, err := meter.Int64ObservableGauge(telemetryMeterPrefix+name,
			metric.WithDescription(desc), metric.WithUnit("{calendar}"))
		if err != nil {
			return nil, fmt.Errorf("failed to create %s gauge: %w", name, err)
		}
		return g, nil
	}
	desc := fmt.Sprintf(" (active = fetched in the last %d days).", activeCalendarDays)
	active, err := gauge("calendars.active", "Active calendars"+desc)
	if err != nil {
		return err
	}
	games, err := gauge("tracked.games", "Active calendars that include each game"+desc)
	if err != nil {
		return err
	}
	leagues, err := gauge("tracked.leagues",
		fmt.Sprintf("Active calendars that explicitly select each league, top %d per game%s", trackedTopPerGame, desc))
	if err != nil {
		return err
	}
	teams, err := gauge("tracked.teams",
		fmt.Sprintf("Active calendars that explicitly select each team, top %d per game%s", trackedTopPerGame, desc))
	if err != nil {
		return err
	}
	tiers, err := gauge("tracked.tiers", "Active calendars by per-game tier auto-include ceiling"+desc)
	if err != nil {
		return err
	}

	cache := &trackedCache{src: src} //nolint:exhaustruct // zero snapshot means not yet loaded
	_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		snap, loadErr := cache.get(ctx)
		if loadErr != nil {
			logger.Warn("Failed to load tracked selections", zap.Error(loadErr))
			return nil
		}
		o.ObserveInt64(active, snap.calendars)
		for _, set := range []struct {
			gauge metric.Int64ObservableGauge
			obs   []observation
		}{{games, snap.games}, {leagues, snap.leagues}, {teams, snap.teams}, {tiers, snap.tiers}} {
			for _, ob := range set.obs {
				o.ObserveInt64(set.gauge, ob.value, ob.attrs)
			}
		}
		return nil
	}, active, games, leagues, teams, tiers)
	if err != nil {
		return fmt.Errorf("failed to register tracked selections callback: %w", err)
	}
	return nil
}
