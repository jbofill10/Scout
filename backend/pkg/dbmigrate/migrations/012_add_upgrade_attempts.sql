-- 012_add_upgrade_attempts.sql
-- One row per library item the upgrade pass has tried to improve, so an item
-- is not searched for every run: a miss is retried after a week and given up
-- on after three tries. item_key is the notification key (episode tvdb id for
-- series, media id for movies).

CREATE TABLE IF NOT EXISTS UpgradeAttempts (
    item_key TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    season INTEGER,
    episode INTEGER,
    from_quality TEXT,
    to_quality TEXT,
    status TEXT NOT NULL,
    reason TEXT,
    attempts INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Rollback:
-- DROP TABLE IF EXISTS UpgradeAttempts;
