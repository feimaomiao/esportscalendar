-- name: InsertToGames :exec
INSERT INTO games (id, name, slug) VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug;

-- name: InsertToLeagues :exec
INSERT INTO leagues (id, name, slug, image_link, game_id) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    image_link = EXCLUDED.image_link,
    game_id = EXCLUDED.game_id;

-- name: InsertToSeries :exec
INSERT INTO series (id, name, slug, game_id, league_id) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    game_id = EXCLUDED.game_id,
    league_id = EXCLUDED.league_id;

-- name: InsertToTournaments :exec
INSERT INTO tournaments (id,name, slug,tier, game_id, league_id, serie_id) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    tier = EXCLUDED.tier,
    game_id = EXCLUDED.game_id,
    league_id = EXCLUDED.league_id,
    serie_id = EXCLUDED.serie_id;

-- name: InsertToMatches :exec
INSERT INTO matches (id, name, slug, finished, expected_start_time, actual_game_time, team1_id, team1_score, team2_id, team2_score, amount_of_games, game_id, league_id, series_id, tournament_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    finished = EXCLUDED.finished,
    expected_start_time = EXCLUDED.expected_start_time,
    actual_game_time = EXCLUDED.actual_game_time,
    team1_id = EXCLUDED.team1_id,
    team1_score = EXCLUDED.team1_score,
    team2_id = EXCLUDED.team2_id,
    team2_score = EXCLUDED.team2_score,
    amount_of_games = EXCLUDED.amount_of_games,
    game_id = EXCLUDED.game_id,
    league_id = EXCLUDED.league_id,
    series_id = EXCLUDED.series_id,
    tournament_id = EXCLUDED.tournament_id;

-- name: InsertToTeams :exec
INSERT INTO teams (id, name, slug, acronym, image_link, game_id) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    acronym = EXCLUDED.acronym,
    image_link = EXCLUDED.image_link,
    game_id = EXCLUDED.game_id;

-- name: GameExist :one
SELECT COUNT(*) FROM games WHERE id = $1;

-- name: LeagueExist :one
SELECT COUNT(*) FROM leagues WHERE id = $1;

-- name: SeriesExist :one
SELECT COUNT(*) FROM series WHERE id = $1;

-- name: TournamentExist :one
SELECT COUNT(*) FROM tournaments WHERE id = $1;

-- name: MatchExist :one
SELECT COUNT(*) FROM matches WHERE id = $1;

-- name: TeamExist :one
SELECT COUNT(*) FROM teams WHERE id = $1;


-- name: GetAllGames :many
SELECT id, name, slug FROM games WHERE (id != 14) ORDER BY id ASC;

-- name: GetSeriesByGameID :many
SELECT id, name, slug, game_id, league_id FROM series WHERE game_id = $1 ORDER BY name ASC;

-- name: GetLeaguesByGameID :many
SELECT l.id, l.name, l.slug, l.game_id, l.image_link, MIN(t.tier) as min_tier
FROM LEAGUES l
LEFT JOIN TOURNAMENTS t ON l.id = t.league_id
WHERE l.game_id = $1
GROUP BY l.id, l.name, l.slug, l.game_id, l.image_link
ORDER BY MIN(t.tier) ASC, l.name ASC;

-- name: GetTeamsByGameID :many
SELECT id, name, slug, acronym, image_link, game_id
FROM teams
WHERE game_id = $1
ORDER BY name ASC;

-- name: GetMatchesBySelections :many
SELECT
    m.id,
    m.name,
    m.slug,
    m.expected_start_time,
    m.finished,
    m.team1_id,
    m.team2_id,
    m.team1_score,
    m.team2_score,
    m.amount_of_games,
    m.game_id,
    m.league_id,
    m.series_id,
    m.tournament_id,
    g.name as game_name,
    l.name as league_name,
    t1.name as team1_name,
    t1.acronym as team1_acronym,
    t1.image_link as team1_image,
    t2.name as team2_name,
    t2.acronym as team2_acronym,
    t2.image_link as team2_image
FROM matches m
INNER JOIN games g ON m.game_id = g.id
INNER JOIN leagues l ON m.league_id = l.id
INNER JOIN teams t1 ON m.team1_id = t1.id
INNER JOIN teams t2 ON m.team2_id = t2.id
WHERE
    m.expected_start_time >= NOW() - INTERVAL '7 days'
    AND m.expected_start_time <= NOW() + INTERVAL '7 days'
    AND m.game_id = ANY(sqlc.arg(game_ids)::int[])
    AND (
        (CARDINALITY(sqlc.arg(league_ids)::int[]) = 0 OR m.league_id = ANY(sqlc.arg(league_ids)::int[]))
        OR (CARDINALITY(sqlc.arg(team_ids)::int[]) = 0 OR m.team1_id = ANY(sqlc.arg(team_ids)::int[]) OR m.team2_id = ANY(sqlc.arg(team_ids)::int[]))
    )
ORDER BY m.expected_start_time ASC;