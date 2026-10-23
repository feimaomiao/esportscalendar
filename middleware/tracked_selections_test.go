package middleware

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

func TestCountSelections(t *testing.T) {
	t.Parallel()
	rows := [][]byte{
		// New format: wrapper, league listed twice, explicit tier.
		[]byte(`{"selections":{"1":{"leagues":[10,10,11],"teams":[100],"maxTier":3}},"hideScores":true}`),
		// Old format: no wrapper, no maxTier (defaults to A), tier off for game 2.
		[]byte(`{"1":{"leagues":[10],"teams":[100,101]},"2":{"teams":[200],"maxTier":0}}`),
		// Stray non-game keys are ignored.
		[]byte(`{"selections":{"x":{},"-1":{},"1":{}}}`),
		[]byte(`not json`),
	}
	got := countSelections(rows)

	if got.calendars != 3 {
		t.Errorf("calendars = %d, want 3", got.calendars)
	}
	wantGames := map[int32]int64{1: 3, 2: 1}
	for id, want := range wantGames {
		if got.games[id] != want {
			t.Errorf("games[%d] = %d, want %d", id, got.games[id], want)
		}
	}
	if len(got.games) != len(wantGames) {
		t.Errorf("games = %v, want %v", got.games, wantGames)
	}
	wantLeagues := map[gameItem]int64{{1, 10}: 2, {1, 11}: 1}
	for k, want := range wantLeagues {
		if got.leagues[k] != want {
			t.Errorf("leagues[%v] = %d, want %d", k, got.leagues[k], want)
		}
	}
	wantTeams := map[gameItem]int64{{1, 100}: 2, {1, 101}: 1, {2, 200}: 1}
	for k, want := range wantTeams {
		if got.teams[k] != want {
			t.Errorf("teams[%v] = %d, want %d", k, got.teams[k], want)
		}
	}
	wantTiers := map[gameTier]int64{
		{1, 3}:              1,
		{1, defaultMaxTier}: 2,
		{2, tierOff}:        1,
	}
	for k, want := range wantTiers {
		if got.tiers[k] != want {
			t.Errorf("tiers[%v] = %d, want %d (all: %v)", k, got.tiers[k], want, got.tiers)
		}
	}
}

func TestTopPerGame(t *testing.T) {
	t.Parallel()
	counts := map[gameItem]int64{
		{1, 5}: 3, {1, 4}: 3, {1, 9}: 7, {1, 2}: 1,
		{2, 8}: 1,
	}
	got := topPerGame(counts, 2)
	want := []itemCount{
		{gameItem{1, 9}, 7},
		{gameItem{1, 4}, 3}, // ties broken by lower ID
		{gameItem{2, 8}, 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestTierLabel(t *testing.T) {
	t.Parallel()
	for tier, want := range map[int32]string{0: "off", 1: "S", 2: "A", 5: "D", 9: "9"} {
		if got := tierLabel(tier); got != want {
			t.Errorf("tierLabel(%d) = %q, want %q", tier, got, want)
		}
	}
}

type fakeTrackedSource struct {
	rows    [][]byte
	err     error
	queries int
}

func (f *fakeTrackedSource) GetActiveURLMappingSelections(_ context.Context, days int32) ([][]byte, error) {
	f.queries++
	if days != activeCalendarDays {
		return nil, errors.New("unexpected window")
	}
	return f.rows, f.err
}

func (f *fakeTrackedSource) GetAllGames(context.Context) ([]dbtypes.Game, error) {
	return []dbtypes.Game{{ID: 1, Name: "League of Legends"}}, nil
}

func (f *fakeTrackedSource) GetLeagueNamesByIDs(
	_ context.Context,
	ids []int32,
) ([]dbtypes.GetLeagueNamesByIDsRow, error) {
	var out []dbtypes.GetLeagueNamesByIDsRow
	for _, id := range ids {
		if id == 10 {
			out = append(out, dbtypes.GetLeagueNamesByIDsRow{ID: 10, Name: "LEC", GameID: 1})
		}
	}
	return out, nil
}

func (f *fakeTrackedSource) GetTeamNamesByIDs(_ context.Context, ids []int32) ([]dbtypes.GetTeamNamesByIDsRow, error) {
	var out []dbtypes.GetTeamNamesByIDsRow
	for _, id := range ids {
		if id == 100 {
			out = append(out, dbtypes.GetTeamNamesByIDsRow{ID: 100, Name: "G2 Esports", GameID: 1})
		}
	}
	return out, nil
}

func newTrackedTelemetry(t *testing.T, src TrackedSource) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if _, err := NewTelemetry(provider.Meter("test"), nil, src, zaptest.NewLogger(t)); err != nil {
		t.Fatalf("NewTelemetry: %v", err)
	}
	return reader
}

// gaugePoints flattens a gauge into "attr=val,attr=val" -> value.
func gaugePoints(t *testing.T, m metricdata.Metrics) map[string]int64 {
	t.Helper()
	g, ok := m.Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatalf("%s is not an int64 gauge", m.Name)
	}
	out := map[string]int64{}
	for _, dp := range g.DataPoints {
		out[dp.Attributes.Encoded(attribute.DefaultEncoder())] = dp.Value
	}
	return out
}

func TestTrackedGauges(t *testing.T) {
	t.Parallel()
	src := &fakeTrackedSource{rows: [][]byte{
		[]byte(`{"selections":{"1":{"leagues":[10],"teams":[100],"maxTier":1}}}`),
		[]byte(`{"selections":{"1":{"leagues":[10,12]},"7":{}}}`),
	}}
	reader := newTrackedTelemetry(t, src)

	want := map[string]map[string]int64{
		"esportscalendar.calendars.active": {"": 2},
		"esportscalendar.tracked.games":    {"game=League of Legends": 2, "game=7": 1},
		"esportscalendar.tracked.leagues": {
			"game=League of Legends,league=LEC,league_id=10": 2,
			"game=League of Legends,league=12,league_id=12":  1,
		},
		"esportscalendar.tracked.teams": {"game=League of Legends,team=G2 Esports,team_id=100": 1},
		"esportscalendar.tracked.tiers": {
			"game=League of Legends,tier=S": 1,
			"game=League of Legends,tier=A": 1,
			"game=7,tier=A":                 1,
		},
	}
	for name, wantPoints := range want {
		got := gaugePoints(t, collect(t, reader, name))
		if len(got) != len(wantPoints) {
			t.Errorf("%s = %v, want %v", name, got, wantPoints)
			continue
		}
		for k, v := range wantPoints {
			if got[k] != v {
				t.Errorf("%s{%s} = %d, want %d (all: %v)", name, k, got[k], v, got)
			}
		}
	}

	// The snapshot is cached: five collections above, one query.
	if src.queries != 1 {
		t.Errorf("queries = %d, want 1 (snapshot should be cached)", src.queries)
	}
}

func TestTrackedGauges_SkipOnError(t *testing.T) {
	t.Parallel()
	src := &fakeTrackedSource{err: errors.New("db down")}
	reader := newTrackedTelemetry(t, src)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok && len(g.DataPoints) > 0 {
				t.Fatalf("%s reported %d points despite DB error", m.Name, len(g.DataPoints))
			}
		}
	}

	// Failures aren't cached: the next collection retries.
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if src.queries != 2 {
		t.Errorf("queries = %d, want 2 (failed refresh should retry)", src.queries)
	}
}
