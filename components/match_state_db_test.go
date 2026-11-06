//go:build dbtest

package components

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/feimaomiao/esportscalendar/dbtypes"
	"github.com/feimaomiao/esportscalendar/internal/testdb"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) { testdb.RunMain(m, &pool) }

func seedMatchCombinations(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`TRUNCATE games, leagues, series, tournaments, matches, teams CASCADE`,
		`INSERT INTO games (id, name, slug) VALUES (1, 'Game', 'game')`,
		`INSERT INTO leagues (id, name, slug, game_id) VALUES (10, 'League', 'league', 1)`,
		`INSERT INTO series (id, name, slug, game_id, league_id) VALUES (100, 'Series', 'series', 1, 10)`,
		`INSERT INTO tournaments (id, name, slug, tier, game_id, league_id, serie_id)
			VALUES (1000, 'Tournament', 'tournament', 1, 1, 10, 100)`,
		`INSERT INTO matches (id, name, slug, finished, expected_start_time, actual_game_time,
			team1_id, team1_score, team2_id, team2_score, amount_of_games, is_live, status, forfeit,
			game_id, league_id, series_id, tournament_id)
		SELECT row_number() OVER (), 'Match', 'match', finished, NOW() + start_offset, 0, 0, 0, 0, 0, 1,
			is_live, status, forfeit, 1, 10, 100, 1000
		FROM (VALUES (NULL::text), ('not_started'), ('running'), ('finished'), ('canceled'),
				('removed'), ('postponed'), ('new_upstream_value')) AS s(status)
		CROSS JOIN (VALUES (false), (true)) AS f(finished)
		CROSS JOIN (VALUES (false), (true)) AS l(is_live)
		CROSS JOIN (VALUES (false), (true)) AS w(forfeit)
		CROSS JOIN (VALUES (INTERVAL '-48 hours'), (INTERVAL '-3 hours'), (INTERVAL '3 hours'), (NULL::interval))
			AS o(start_offset)`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// TestFixturesSectionsMatchDerivedState runs every combination of the columns
// through the real queries. /fixtures is the three lists concatenated, so a row
// no list claims silently disappears and a row two lists claim shows up twice.
func TestFixturesSectionsMatchDerivedState(t *testing.T) {
	seedMatchCombinations(t)
	ctx := context.Background()
	db := dbtypes.New(pool)
	games, leagues, noTiers := []int32{1}, []int32{10}, []int32{0}
	const limit = 1000

	past, err := db.GetPastMatchesBySelections(ctx, dbtypes.GetPastMatchesBySelectionsParams{
		GameIds: games, LeagueIds: leagues, TeamIds: nil, MaxTiers: noTiers, LimitCount: limit,
	})
	if err != nil {
		t.Fatalf("past: %v", err)
	}
	ongoing, err := db.GetOngoingMatchesBySelections(ctx, dbtypes.GetOngoingMatchesBySelectionsParams{
		GameIds: games, LeagueIds: leagues, TeamIds: nil, MaxTiers: noTiers, LimitCount: limit,
	})
	if err != nil {
		t.Fatalf("ongoing: %v", err)
	}
	future, err := db.GetFutureMatchesBySelections(ctx, dbtypes.GetFutureMatchesBySelectionsParams{
		GameIds: games, LeagueIds: leagues, TeamIds: nil, MaxTiers: noTiers, LimitCount: limit,
	})
	if err != nil {
		t.Fatalf("future: %v", err)
	}

	type placed struct {
		section string
		row     dbtypes.GetFutureMatchesBySelectionsRow
	}
	var all []placed
	for _, r := range past {
		all = append(all, placed{"past", dbtypes.GetFutureMatchesBySelectionsRow(r)})
	}
	for _, r := range ongoing {
		all = append(all, placed{"ongoing", dbtypes.GetFutureMatchesBySelectionsRow(r)})
	}
	for _, r := range future {
		all = append(all, placed{"future", r})
	}

	now := time.Now()
	sections := make(map[int32]string)
	for _, p := range all {
		label := fmt.Sprintf("match %d (status=%q finished=%t live=%t)",
			p.row.ID, p.row.Status.String, p.row.Finished, p.row.IsLive)
		if prior, dup := sections[p.row.ID]; dup {
			t.Errorf("%s is in both %s and %s", label, prior, p.section)
		}
		sections[p.row.ID] = p.section

		state := DeriveMatchState(MatchFacts{
			Status: p.row.Status, Finished: p.row.Finished, IsLive: p.row.IsLive,
			Forfeit: p.row.Forfeit, Start: p.row.ExpectedStartTime,
		}, now)
		if (p.section == "ongoing") != (state == MatchOngoing) {
			t.Errorf("%s: section %s disagrees with state %s", label, p.section, state)
		}
		switch {
		case p.section == "future" && (state == MatchFinished || state == MatchAwaitingResult):
			t.Errorf("%s: %s can't be upcoming", label, state)
		case p.section == "past" && state == MatchUpcoming:
			t.Errorf("%s: %s can't be past", label, state)
		}
	}

	rows, err := pool.Query(ctx, `SELECT id FROM matches WHERE expected_start_time IS NOT NULL`)
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	defer rows.Close()
	checked := 0
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		checked++
		if _, ok := sections[id]; !ok {
			t.Errorf("match %d has a start time but is in no section", id)
		}
	}
	if checked == 0 {
		t.Fatal("seeded no rows")
	}
}
