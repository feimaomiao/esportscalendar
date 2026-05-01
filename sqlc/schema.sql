-- =============================================================================
-- Canonical schema for the shared esports DB.
-- stalka writes the data and executes this file on startup
-- (see stalka/main.go: db.Exec(ctx, schema)).
-- esportscalendar reads the data and uses this file only for sqlc codegen.
-- Keep stalka/static/schema.sql and esportscalendar/sqlc/schema.sql
-- byte-identical, otherwise the two services drift.
-- =============================================================================

CREATE TABLE IF NOT EXISTS GAMES(
    id INTEGER PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255)
);

CREATE TABLE IF NOT EXISTS LEAGUES(
    id INTEGER PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255),
    game_id INT NOT NULL,
    image_link VARCHAR(255),
    FOREIGN KEY (game_id) REFERENCES GAMES(id)
);

CREATE TABLE IF NOT EXISTS SERIES(
    id INTEGER PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255),
    game_id INT NOT NULL,
    league_id INT NOT NULL,
    FOREIGN KEY (game_id) REFERENCES GAMES(id),
    FOREIGN KEY (league_id) REFERENCES LEAGUES(id)
);

CREATE TABLE IF NOT EXISTS TOURNAMENTS(
    id INTEGER PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255),
    tier INT,
    game_id INT NOT NULL,
    league_id INT NOT NULL,
    serie_id INT NOT NULL,
    FOREIGN KEY (game_id) REFERENCES GAMES(id),
    FOREIGN KEY (serie_id) REFERENCES SERIES(id),
    FOREIGN KEY (league_id) REFERENCES LEAGUES(id)
);


CREATE TABLE IF NOT EXISTS MATCHES(
    id INTEGER PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255),
    finished BOOLEAN NOT NULL,
    expected_start_time TIMESTAMP,
    actual_game_time FLOAT NOT NULL,
    team1_id INT NOT NULL,
    team1_score INT NOT NULL,
    team2_id INT NOT NULL,
    team2_score INT NOT NULL,
    amount_of_games INT NOT NULL,
    is_live BOOLEAN NOT NULL DEFAULT false,
    stream_url TEXT,
    game_id INT NOT NULL,
    league_id INT NOT NULL,
    series_id INT NOT NULL,
    tournament_id INT NOT NULL,
    FOREIGN KEY (game_id) REFERENCES GAMES(id),
    FOREIGN KEY (league_id) REFERENCES LEAGUES(id),
    FOREIGN KEY (series_id) REFERENCES SERIES(id),
    FOREIGN KEY (tournament_id) REFERENCES TOURNAMENTS(id)
);

CREATE TABLE IF NOT EXISTS TEAMS(
    id INTEGER PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255),
    acronym VARCHAR(255),
    image_link VARCHAR(255),
    game_id INT NOT NULL,
    FOREIGN KEY (game_id) REFERENCES GAMES(id)
);

CREATE TABLE IF NOT EXISTS URL_MAPPINGS(
    hashed_key VARCHAR(16) NOT NULL PRIMARY KEY,
    value_list JSON NOT NULL,
    access_count INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    accessed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- =============================================================================
-- Migration appendix: add columns to existing deployments that pre-date them.
-- New deployments hit the CREATE TABLE definitions above and skip these.
-- Append new ALTERs here when adding columns; never edit the CREATE TABLE
-- alone or older databases will miss the column.
-- =============================================================================
ALTER TABLE MATCHES ADD COLUMN IF NOT EXISTS is_live BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE MATCHES ADD COLUMN IF NOT EXISTS stream_url TEXT;
