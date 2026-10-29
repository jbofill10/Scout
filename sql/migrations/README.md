# Migrations moved

Schema migrations now live in `backend/pkg/dbmigrate/migrations/` and are applied
automatically by the webserver and torrenter at startup. See
`backend/pkg/dbmigrate/README.md` and the "Database Migrations" section of
`DEPLOYMENT.md`.

`sql/setup-postgres.sql` still bootstraps a brand new database (docker-compose init).
