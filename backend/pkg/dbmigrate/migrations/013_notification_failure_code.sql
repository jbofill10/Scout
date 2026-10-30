-- 013_notification_failure_code.sql
-- Records the machine-readable failure code next to a failed notification's
-- human reason, so the webserver can tell a transient loss (monitor timeout,
-- dead swarm, torrent removed) that deserves a retry from a permanent one
-- after the torrenter reported it asynchronously.

ALTER TABLE Notifications ADD COLUMN IF NOT EXISTS failure_code TEXT NOT NULL DEFAULT '';
