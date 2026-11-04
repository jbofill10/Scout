-- 015_add_ratings_cache.sql
-- tvdb-proxy's rating answers (the Tomatometer and Metascore from Wikidata,
-- Wikipedia and OMDb, and the IMDb ids resolved through TVDB and Wikidata)
-- lived only in the proxy's memory, so every restart asked every source for
-- every card again: a free OMDb key allows 1,000 lookups a day and the
-- library alone is hundreds of titles. This table keeps each answer, keyed
-- by its source and id, with when it stops being current. An expired row
-- still stands in until a fresh answer lands, so the library's scores
-- build up and show at once after a restart.

CREATE TABLE IF NOT EXISTS RatingsCache (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ratingscache_expires_at ON RatingsCache (expires_at);
