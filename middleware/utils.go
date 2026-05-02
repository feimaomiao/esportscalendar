package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	"go.uber.org/zap"
)

var (
	errEmptySelections = errors.New("no valid game selections")
	errTooManyGames    = errors.New("too many games selected")
	errTooManyLeagues  = errors.New("too many leagues selected")
	errTooManyTeams    = errors.New("too many teams selected")
)

const (
	// tierOff = 0 disables the auto-include branch in the SQL: a game with
	// maxTier=0 only contributes matches that match its selected leagues or
	// teams, with no tier-based "show me big tournaments" inclusion. Letter
	// tiers map 1=S (best), 2=A, 3=B, 4=C, 5=D (worst). The slider exposes
	// OFF + S–D; "All" was dropped because at slider position D the SQL
	// already matches every tournament-tagged tier.
	tierOff        = 0
	defaultMaxTier = 2 // Default to tier A — auto-includes S+A.
	minTier        = tierOff
	maxTierBound   = 5 // Tier D — loosest auto-include.

	// Per-request cardinality caps to bound DB query cost and reject abuse.
	maxGamesPerRequest   = 50
	maxLeaguesPerRequest = 1000
	maxTeamsPerRequest   = 5000
)

// generateHash creates a consistent hash from the selections JSON.
func generateHash(data []byte) string {
	hash := sha256.Sum256(data)
	// Return first 16 characters for a shorter URL
	return hex.EncodeToString(hash[:])[:16]
}

// parseSelections extracts game IDs, league IDs, team IDs, and per-game max
// tiers from selections JSON. The returned maxTiers slice is index-aligned
// with gameIDs — element i is the tier ceiling for game i. Games without an
// explicit maxTier in the payload get defaultMaxTier. Index alignment is
// load-bearing: the tier-filter SQL looks up each match's ceiling via
// array_position(game_ids, m.game_id) → max_tiers[idx], so any drift between
// the two slices would route a match to the wrong game's filter.
func parseSelections(
	selections map[string]any,
	logger *zap.Logger,
) ([]int32, []int32, []int32, []int32) {
	var gameIDs, leagueIDs, teamIDs, maxTiers []int32

	for gameIDStr, selectionData := range selections {
		gameID, parseErr := strconv.ParseInt(gameIDStr, 10, 32)
		if parseErr != nil {
			logger.Warn("Invalid game ID", zap.String("game_id_str", gameIDStr), zap.Error(parseErr))
			continue
		}
		if gameID <= 0 {
			continue
		}
		gameIDs = append(gameIDs, int32(gameID))
		gameLeagues, gameTeams, gameTier := parseGameSelection(selectionData)
		leagueIDs = append(leagueIDs, gameLeagues...)
		teamIDs = append(teamIDs, gameTeams...)
		maxTiers = append(maxTiers, gameTier)
	}

	return gameIDs, leagueIDs, teamIDs, maxTiers
}

// parseGameSelection extracts the leagues, teams, and tier ceiling for one
// game from the untyped JSON-decoded selection blob. Returns defaultMaxTier
// when the blob is malformed or omits maxTier so callers can append a
// matching maxTiers entry unconditionally.
func parseGameSelection(data any) ([]int32, []int32, int32) {
	tier := int32(defaultMaxTier)
	selectionMap, ok := data.(map[string]any)
	if !ok {
		return nil, nil, tier
	}
	leagues := extractInt32IDs(selectionMap["leagues"])
	teams := extractInt32IDs(selectionMap["teams"])
	if tierValue, tierOk := selectionMap["maxTier"].(float64); tierOk {
		t := int32(tierValue)
		if t < minTier {
			t = minTier
		}
		if t > maxTierBound {
			t = maxTierBound
		}
		tier = t
	}
	return leagues, teams, tier
}

// extractInt32IDs pulls a list of positive int32 IDs from the untyped JSON
// array under a single key. Non-numeric, non-positive, and overflow values
// are dropped silently — the caller treats anything missing as "no IDs".
func extractInt32IDs(raw any) []int32 {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]int32, 0, len(arr))
	for _, v := range arr {
		f, isNum := v.(float64)
		if !isNum {
			continue
		}
		if f <= 0 || f > 2_147_483_647 {
			continue
		}
		out = append(out, int32(f))
	}
	return out
}

// defaultMaxTiers builds a per-game tier slice of length n filled with
// defaultMaxTier. Used by handlers that render initial defaults (no client-
// supplied selections payload), where every game should fall back to the
// same A-tier-and-stricter ceiling.
func defaultMaxTiers(n int) []int32 {
	if n == 0 {
		return nil
	}
	out := make([]int32, n)
	for i := range out {
		out[i] = int32(defaultMaxTier)
	}
	return out
}

// validateSelections enforces per-request cardinality caps. Call after
// parseSelections to bound DB query cost.
func validateSelections(gameIDs, leagueIDs, teamIDs []int32) error {
	if len(gameIDs) == 0 {
		return errEmptySelections
	}
	if len(gameIDs) > maxGamesPerRequest {
		return errTooManyGames
	}
	if len(leagueIDs) > maxLeaguesPerRequest {
		return errTooManyLeagues
	}
	if len(teamIDs) > maxTeamsPerRequest {
		return errTooManyTeams
	}
	return nil
}
