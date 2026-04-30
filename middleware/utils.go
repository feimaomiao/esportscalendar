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
	defaultMaxTier = 2 // Default to tier A (tier 2)
	minTier        = 1 // Tier S
	maxTierBound   = 6 // "All"

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

// parseSelections extracts game IDs, league IDs, team IDs, and max tier from selections JSON.
//
//nolint:gocognit // Complexity comes from defensively traversing untyped JSON.
func parseSelections(
	selections map[string]any,
	logger *zap.Logger,
) ([]int32, []int32, []int32, int32) {
	var gameIDs, leagueIDs, teamIDs []int32
	maxTier := int32(defaultMaxTier)

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

		selectionMap, ok := selectionData.(map[string]any)
		if !ok {
			continue
		}

		// Extract league IDs
		if leagues, leaguesOk := selectionMap["leagues"].([]any); leaguesOk {
			for _, league := range leagues {
				leagueID, leagueOk := league.(float64)
				if !leagueOk {
					continue
				}
				if leagueID <= 0 || leagueID > 2_147_483_647 {
					continue
				}
				leagueIDs = append(leagueIDs, int32(leagueID))
			}
		}

		// Extract team IDs
		if teams, teamsOk := selectionMap["teams"].([]any); teamsOk {
			for _, team := range teams {
				teamID, teamOk := team.(float64)
				if !teamOk {
					continue
				}
				if teamID <= 0 || teamID > 2_147_483_647 {
					continue
				}
				teamIDs = append(teamIDs, int32(teamID))
			}
		}

		// Extract max tier — clamp to the valid range [1, 6].
		if tierValue, tierOk := selectionMap["maxTier"].(float64); tierOk {
			tier := int32(tierValue)
			if tier < minTier {
				tier = minTier
			}
			if tier > maxTierBound {
				tier = maxTierBound
			}
			if tier > maxTier {
				maxTier = tier
			}
		}
	}

	return gameIDs, leagueIDs, teamIDs, maxTier
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
