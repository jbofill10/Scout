-- 011_add_notification_targets.sql
-- Outbound notifications: targets (webhook, ntfy, discord) that receive
-- download outcomes, and a marker on Notifications recording which terminal
-- status has been delivered, so each transition is sent exactly once.
--
-- The marker is backfilled for rows that are already terminal when the
-- column is first added, so deploying this does not replay history.

CREATE TABLE IF NOT EXISTS NotificationTargets (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    url TEXT NOT NULL,
    token TEXT,
    events TEXT[] NOT NULL DEFAULT ARRAY['completed', 'failed'],
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_sent_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $mig$
DECLARE
    first_run BOOLEAN;
BEGIN
    first_run := NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'notifications' AND column_name = 'notified_status'
    );

    ALTER TABLE Notifications ADD COLUMN IF NOT EXISTS notified_status TEXT;

    IF first_run THEN
        UPDATE Notifications SET notified_status = status
        WHERE status IN ('completed', 'failed', 'cancelled');
    END IF;
END $mig$;

CREATE INDEX IF NOT EXISTS idx_notifications_pending_delivery
ON Notifications (updated_at)
WHERE status IN ('completed', 'failed', 'cancelled');

-- Rollback:
-- DROP INDEX IF EXISTS idx_notifications_pending_delivery;
-- ALTER TABLE Notifications DROP COLUMN IF EXISTS notified_status;
-- DROP TABLE IF EXISTS NotificationTargets;
