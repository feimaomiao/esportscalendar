package components

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestDeriveMatchState(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	past := pgtype.Timestamp{Time: now.Add(-time.Hour), Valid: true}
	future := pgtype.Timestamp{Time: now.Add(time.Hour), Valid: true}
	exactlyNow := pgtype.Timestamp{Time: now, Valid: true}
	noStart := pgtype.Timestamp{}
	longAgo := pgtype.Timestamp{Time: now.Add(-LiveStaleAfter), Valid: true}
	justInside := pgtype.Timestamp{Time: now.Add(-LiveStaleAfter + time.Minute), Valid: true}
	status := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
	null := pgtype.Text{}

	cases := []struct {
		name     string
		status   pgtype.Text
		finished bool
		isLive   bool
		forfeit  bool
		start    pgtype.Timestamp
		want     MatchState
	}{
		// NULL status: every row until stalka refetches it.
		{name: "null, future", status: null, start: future, want: MatchUpcoming},
		{name: "null, past", status: null, start: past, want: MatchAwaitingResult},
		{name: "null, no start", status: null, start: noStart, want: MatchUpcoming},
		{name: "null, live", status: null, isLive: true, start: past, want: MatchOngoing},
		{name: "null, live before its start", status: null, isLive: true, start: future, want: MatchOngoing},
		{name: "null, finished", status: null, finished: true, start: past, want: MatchFinished},
		{
			name:     "null, finished beats stale live",
			status:   null,
			finished: true,
			isLive:   true,
			start:    past,
			want:     MatchFinished,
		},
		{name: "null, finished before its start", status: null, finished: true, start: future, want: MatchFinished},

		{name: "not started, future", status: status("not_started"), start: future, want: MatchUpcoming},
		{name: "not started, running late", status: status("not_started"), start: past, want: MatchAwaitingResult},
		{name: "starts right now", status: status("not_started"), start: exactlyNow, want: MatchUpcoming},
		{name: "running", status: status("running"), isLive: true, start: past, want: MatchOngoing},
		{name: "finished", status: status("finished"), finished: true, start: past, want: MatchFinished},
		{
			name:   "unknown status behaves like null",
			status: status("something_new"),
			start:  past,
			want:   MatchAwaitingResult,
		},

		{name: "postponed, future", status: status("postponed"), start: future, want: MatchPostponed},
		{name: "postponed, past", status: status("postponed"), start: past, want: MatchPostponed},
		{name: "postponed but live", status: status("postponed"), isLive: true, start: past, want: MatchOngoing},

		{name: "canceled, past", status: status("canceled"), start: past, want: MatchCancelled},
		{name: "canceled, future", status: status("canceled"), start: future, want: MatchCancelled},
		{
			name:   "canceled beats stale live",
			status: status("canceled"),
			isLive: true,
			start:  past,
			want:   MatchCancelled,
		},
		{
			name:     "canceled beats stale finished",
			status:   status("canceled"),
			finished: true,
			start:    past,
			want:     MatchCancelled,
		},
		{name: "removed", status: status("removed"), start: past, want: MatchCancelled},

		{name: "forfeit", status: status("canceled"), forfeit: true, start: past, want: MatchForfeit},
		{
			name:     "forfeit flag needs a cancellation",
			status:   status("finished"),
			finished: true,
			forfeit:  true,
			start:    past,
			want:     MatchFinished,
		},
		{
			name:    "removed forfeit is just cancelled",
			status:  status("removed"),
			forfeit: true,
			start:   past,
			want:    MatchCancelled,
		},

		// is_live can't be trusted forever: it only changes while stalka polls.
		{name: "live within the limit", status: status("running"), isLive: true, start: justInside, want: MatchOngoing},
		{
			name:   "live past the limit",
			status: status("running"),
			isLive: true,
			start:  longAgo,
			want:   MatchAwaitingResult,
		},
		{name: "live with no start stays live", status: null, isLive: true, start: noStart, want: MatchOngoing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := MatchFacts{
				Status: tc.status, Finished: tc.finished, IsLive: tc.isLive, Forfeit: tc.forfeit, Start: tc.start,
			}
			got := DeriveMatchState(facts, now)
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestForfeitWinner(t *testing.T) {
	one := pgtype.Text{String: "T1", Valid: true}
	two := pgtype.Text{String: "Gen.G", Valid: true}
	cases := []struct {
		name   string
		winner int32
		want   string
	}{
		{name: "no winner recorded", winner: 0, want: ""},
		{name: "first team", winner: 7, want: "T1"},
		{name: "second team", winner: 8, want: "Gen.G"},
		{name: "neither side", winner: 9, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ForfeitWinner(tc.winner, 7, 8, one, two); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
