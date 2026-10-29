# dbmigrate

Applies the SQL files in `migrations/` (embedded into the binary) at service
startup. Used by the webserver and torrenter; tvdb-proxy has no database.

- Naming: `NNN_description.sql`, e.g. `009_add_foo_column.sql`. The version key is
  the full file name. Files run in lexical order; never rename or renumber a file
  that has shipped.
- Applied files are recorded in `schema_migrations`. Each file runs in its own
  transaction together with its version row. A file containing `CONCURRENTLY`
  (outside comments) runs without a transaction.
- Idempotency is required. The first run on an existing database re-applies
  everything to baseline, and `setup-postgres.sql` already creates the final
  schema on fresh databases. Use `IF NOT EXISTS` / `IF EXISTS`, `ON CONFLICT DO
  NOTHING` / `WHERE NOT EXISTS` for backfills, and wrap anything else in a
  `DO $$ ... $$` block that checks `information_schema` first. Do not reference
  columns that later migrations drop outside such a guard.
- Do not put `BEGIN`/`COMMIT` in a file; the runner owns the transaction.
- A `pg_advisory_lock` serialises services starting together.
- `DB_AUTO_MIGRATE=false` skips the runner.
