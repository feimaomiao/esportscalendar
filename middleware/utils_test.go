package middleware

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestGenerateHash(t *testing.T) {
	t.Parallel()

	const expectedLen = 16

	t.Run("deterministic for the same input", func(t *testing.T) {
		t.Parallel()
		a := generateHash([]byte(`{"hello":"world"}`))
		b := generateHash([]byte(`{"hello":"world"}`))
		if a != b {
			t.Fatalf("expected identical hashes, got %q vs %q", a, b)
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		t.Parallel()
		if generateHash([]byte("a")) == generateHash([]byte("b")) {
			t.Fatal("expected different hashes for different inputs")
		}
	})

	t.Run("output length is 16", func(t *testing.T) {
		t.Parallel()
		h := generateHash([]byte(""))
		if len(h) != expectedLen {
			t.Fatalf("hash length = %d, want %d", len(h), expectedLen)
		}
	})

	t.Run("output is lowercase hex", func(t *testing.T) {
		t.Parallel()
		h := generateHash([]byte("anything"))
		if h != strings.ToLower(h) {
			t.Fatalf("hash %q is not lowercase", h)
		}
		for _, r := range h {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				t.Fatalf("hash %q contains non-hex char %q", h, r)
			}
		}
	})
}

func TestValidateSelections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		games   []int32
		leagues []int32
		teams   []int32
		want    error
	}{
		{
			name:    "happy path",
			games:   []int32{1, 2, 3},
			leagues: []int32{10, 20},
			teams:   []int32{100},
			want:    nil,
		},
		{
			name:    "empty games rejected",
			games:   []int32{},
			leagues: []int32{10},
			teams:   []int32{100},
			want:    errEmptySelections,
		},
		{
			name:    "nil games rejected as empty",
			games:   nil,
			leagues: nil,
			teams:   nil,
			want:    errEmptySelections,
		},
		{
			name:    "exactly at games cap is allowed",
			games:   makeIDs(maxGamesPerRequest),
			leagues: []int32{1},
			teams:   []int32{1},
			want:    nil,
		},
		{
			name:    "one over games cap rejected",
			games:   makeIDs(maxGamesPerRequest + 1),
			leagues: []int32{1},
			teams:   []int32{1},
			want:    errTooManyGames,
		},
		{
			name:    "one over leagues cap rejected",
			games:   []int32{1},
			leagues: makeIDs(maxLeaguesPerRequest + 1),
			teams:   []int32{1},
			want:    errTooManyLeagues,
		},
		{
			name:    "one over teams cap rejected",
			games:   []int32{1},
			leagues: []int32{1},
			teams:   makeIDs(maxTeamsPerRequest + 1),
			want:    errTooManyTeams,
		},
		{
			name:    "leagues at cap allowed",
			games:   []int32{1},
			leagues: makeIDs(maxLeaguesPerRequest),
			teams:   []int32{1},
			want:    nil,
		},
		{
			name:    "teams at cap allowed",
			games:   []int32{1},
			leagues: []int32{1},
			teams:   makeIDs(maxTeamsPerRequest),
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := validateSelections(tt.games, tt.leagues, tt.teams)
			if !errors.Is(got, tt.want) {
				t.Fatalf("validateSelections() error = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSelections(t *testing.T) {
	t.Parallel()

	mustParse := func(t *testing.T, raw string) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("test fixture json invalid: %v", err)
		}
		return m
	}

	t.Run("empty selections returns empty slices and default tier", func(t *testing.T) {
		t.Parallel()
		games, leagues, teams, tier := parseSelections(map[string]any{}, zap.NewNop())
		if len(games) != 0 || len(leagues) != 0 || len(teams) != 0 {
			t.Fatalf("expected empty slices, got games=%v leagues=%v teams=%v", games, leagues, teams)
		}
		if tier != defaultMaxTier {
			t.Fatalf("tier = %d, want %d (default)", tier, defaultMaxTier)
		}
	})

	t.Run("happy path with one game", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{
			"1": {
				"leagues": [10, 20, 30],
				"teams": [100, 200],
				"maxTier": 3
			}
		}`)
		games, leagues, teams, tier := parseSelections(input, zap.NewNop())
		if !equalInt32(games, []int32{1}) {
			t.Errorf("games = %v, want [1]", games)
		}
		if !equalSetInt32(leagues, []int32{10, 20, 30}) {
			t.Errorf("leagues = %v, want {10,20,30}", leagues)
		}
		if !equalSetInt32(teams, []int32{100, 200}) {
			t.Errorf("teams = %v, want {100,200}", teams)
		}
		if tier != 3 {
			t.Errorf("tier = %d, want 3", tier)
		}
	})

	t.Run("non-numeric game id is skipped", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{"abc": {"leagues": [1]}, "5": {"leagues": [2]}}`)
		games, leagues, _, _ := parseSelections(input, zap.NewNop())
		if !equalInt32(games, []int32{5}) {
			t.Errorf("games = %v, want [5]", games)
		}
		if !equalInt32(leagues, []int32{2}) {
			t.Errorf("leagues = %v, want [2]", leagues)
		}
	})

	t.Run("non-positive game id is skipped", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{"0": {}, "-1": {}, "7": {}}`)
		games, _, _, _ := parseSelections(input, zap.NewNop())
		if !equalInt32(games, []int32{7}) {
			t.Errorf("games = %v, want [7]", games)
		}
	})

	t.Run("non-positive and overflow league/team ids are dropped", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{
			"1": {
				"leagues": [0, -3, 5, 2147483648],
				"teams":   [-1, 99, 9999999999]
			}
		}`)
		_, leagues, teams, _ := parseSelections(input, zap.NewNop())
		if !equalInt32(leagues, []int32{5}) {
			t.Errorf("leagues = %v, want [5]", leagues)
		}
		if !equalInt32(teams, []int32{99}) {
			t.Errorf("teams = %v, want [99]", teams)
		}
	})

	t.Run("non-numeric league/team entries are skipped", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{
			"1": {
				"leagues": ["nope", true, 7, null],
				"teams":   [{"id":1}, 8]
			}
		}`)
		_, leagues, teams, _ := parseSelections(input, zap.NewNop())
		if !equalInt32(leagues, []int32{7}) {
			t.Errorf("leagues = %v, want [7]", leagues)
		}
		if !equalInt32(teams, []int32{8}) {
			t.Errorf("teams = %v, want [8]", teams)
		}
	})

	t.Run("maxTier below 1 is clamped to 1, but default tier 2 wins", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{"1": {"maxTier": 0}}`)
		_, _, _, tier := parseSelections(input, zap.NewNop())
		// tier in payload clamps to 1 (minTier), but defaultMaxTier=2 is higher,
		// and parseSelections only updates maxTier when the parsed tier is greater.
		if tier != defaultMaxTier {
			t.Errorf("tier = %d, want %d (default wins)", tier, defaultMaxTier)
		}
	})

	t.Run("maxTier above 6 is clamped to 6", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{"1": {"maxTier": 99}}`)
		_, _, _, tier := parseSelections(input, zap.NewNop())
		if tier != maxTierBound {
			t.Errorf("tier = %d, want %d", tier, maxTierBound)
		}
	})

	t.Run("multiple games take the max maxTier across them", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{
			"1": {"maxTier": 2},
			"2": {"maxTier": 5},
			"3": {"maxTier": 3}
		}`)
		_, _, _, tier := parseSelections(input, zap.NewNop())
		if tier != 5 {
			t.Errorf("tier = %d, want 5 (highest)", tier)
		}
	})

	t.Run("game with non-object selection data still records the game id", func(t *testing.T) {
		t.Parallel()
		input := mustParse(t, `{"4": "garbage"}`)
		games, leagues, teams, _ := parseSelections(input, zap.NewNop())
		if !equalInt32(games, []int32{4}) {
			t.Errorf("games = %v, want [4]", games)
		}
		if len(leagues) != 0 || len(teams) != 0 {
			t.Errorf("expected no leagues/teams, got %v / %v", leagues, teams)
		}
	})
}

func makeIDs(n int) []int32 {
	out := make([]int32, n)
	for i := range out {
		out[i] = int32(i + 1)
	}
	return out
}

func equalInt32(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// equalSetInt32 compares slices ignoring order — parseSelections walks a Go map
// whose iteration order is randomized, so league/team ordering isn't stable.
func equalSetInt32(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[int32]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}
