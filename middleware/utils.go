package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"
)

// generateHash creates a consistent hash from the selections JSON.
func generateHash(data []byte) string {
	hash := sha256.Sum256(data)
	// Return first 16 characters for a shorter URL
	return hex.EncodeToString(hash[:])[:16]
}

// writeJSON writes a JSON response and logs errors.
func writeJSON(w http.ResponseWriter, data any, logger *zap.Logger) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

// parseSelections extracts game IDs, league IDs, team IDs, and max tier from selections JSON.
func parseSelections(
	selections map[string]any,
	logger *zap.Logger,
) ([]int32, []int32, []int32, int32) {
	var gameIDs, leagueIDs, teamIDs []int32
	maxTier := int32(1) // Default to tier 1

	for gameIDStr, selectionData := range selections {
		gameID, parseErr := strconv.ParseInt(gameIDStr, 10, 32)
		if parseErr != nil {
			logger.Warn("Invalid game ID", zap.String("game_id_str", gameIDStr), zap.Error(parseErr))
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
				if leagueID, leagueOk := league.(float64); leagueOk {
					leagueIDs = append(leagueIDs, int32(leagueID))
				}
			}
		}

		// Extract team IDs
		if teams, teamsOk := selectionMap["teams"].([]any); teamsOk {
			for _, team := range teams {
				if teamID, teamOk := team.(float64); teamOk {
					teamIDs = append(teamIDs, int32(teamID))
				}
			}
		}

		// Extract max tier (use the maximum across all games to be most inclusive)
		if tierValue, tierOk := selectionMap["maxTier"].(float64); tierOk {
			tier := int32(tierValue)
			logger.Debug("Tier selection for game",
				zap.Int32("game_id", int32(gameID)),
				zap.Int32("selected_tier", tier),
				zap.Int32("current_max_tier", maxTier))
			if tier > maxTier {
				maxTier = tier
				logger.Debug("Updated maxTier", zap.Int32("new_max_tier", maxTier))
			}
		}
	}

	logger.Debug("Final tier selection", zap.Int32("max_tier", maxTier))
	return gameIDs, leagueIDs, teamIDs, maxTier
}
