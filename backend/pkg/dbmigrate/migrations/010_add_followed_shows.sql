-- 010_add_followed_shows.sql
-- Shows the user follows: the webserver re-checks TVDB for them on a timer
-- and requests episodes that are new to Scout, so a new season is picked up
-- without the show being requested again. media is the tvdb.Media snapshot
-- (without episodes) the request path is fed with.

CREATE TABLE IF NOT EXISTS FollowedShows (
    tvdb_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    poster_url TEXT,
    media JSONB NOT NULL,
    followed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    include_missing BOOLEAN NOT NULL DEFAULT false,
    last_checked_at TIMESTAMPTZ,
    last_check_error TEXT,
    episodes_added INTEGER NOT NULL DEFAULT 0
);

-- Rollback:
-- DROP TABLE IF EXISTS FollowedShows;
