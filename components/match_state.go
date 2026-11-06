package components

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// MatchState is the single answer to "what is happening with this match", for
// every page and feed. Its values double as the data-status attribute in markup.
type MatchState string

const (
	MatchUpcoming       MatchState = "upcoming"
	MatchOngoing        MatchState = "ongoing"
	MatchAwaitingResult MatchState = "awaiting"
	MatchFinished       MatchState = "finished"
	MatchPostponed      MatchState = "postponed"
	MatchCancelled      MatchState = "cancelled"
	MatchForfeit        MatchState = "forfeit"
)

// LiveStaleAfter bounds how long is_live is believed after a match's start. The
// flag only changes while stalka is polling, so during a scraper outage it would
// otherwise read "ongoing" indefinitely. No match runs this long past its start.
const LiveStaleAfter = 24 * time.Hour

// Values of matches.status that change a match's state. "removed" is set by
// stalka when PandaScore no longer has the match; the rest are PandaScore's.
const (
	statusCanceled  = "canceled"
	statusRemoved   = "removed"
	statusPostponed = "postponed"
)

// MatchFacts are the columns stalka writes that a match's state depends on.
type MatchFacts struct {
	Status   pgtype.Text
	Finished bool
	IsLive   bool
	Forfeit  bool
	Start    pgtype.Timestamp
}

// DeriveMatchState combines those columns into one state. A NULL status means
// stalka hasn't refetched the row since the column was added, and such a row is
// judged on the other columns alone. The fixtures queries apply the same rules
// in SQL: see GetOngoingMatchesBySelections.
func DeriveMatchState(facts MatchFacts, now time.Time) MatchState {
	status := facts.Status.String
	live := facts.IsLive && (!facts.Start.Valid || now.Sub(facts.Start.Time) < LiveStaleAfter)
	switch {
	case status == statusCanceled && facts.Forfeit:
		return MatchForfeit
	case status == statusCanceled || status == statusRemoved:
		return MatchCancelled
	case facts.Finished:
		return MatchFinished
	case live:
		return MatchOngoing
	case status == statusPostponed:
		return MatchPostponed
	case !facts.Start.Valid || !facts.Start.Time.Before(now):
		return MatchUpcoming
	default:
		// Past its start, neither live nor finished: either it ended and stalka
		// hasn't written the result yet, or it's running late.
		return MatchAwaitingResult
	}
}

// ForfeitWinner names the team a forfeited match was awarded to, or "" if the
// winner isn't one of the two sides stored on the row.
func ForfeitWinner(winnerID, team1ID, team2ID int32, team1Name, team2Name pgtype.Text) string {
	switch winnerID {
	case 0:
		return ""
	case team1ID:
		return team1Name.String
	case team2ID:
		return team2Name.String
	}
	return ""
}
