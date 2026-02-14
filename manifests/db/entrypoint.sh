#!/bin/bash
set -e

# Map DB_* env vars to PG* env vars for psql
export PGHOST="${DB_HOST}"
export PGPORT="${DB_PORT}"
export PGUSER="${DB_USER}"
export PGPASSWORD="${DB_PASSWORD}"
export PGDATABASE="${DB_NAME}"
export PGSSLMODE="${DB_SSLMODE:-require}"
[ -n "${DB_SSLROOTCERT}" ] && export PGSSLROOTCERT="${DB_SSLROOTCERT}"

echo "Connecting to ${PGHOST}:${PGPORT}/${PGDATABASE} (sslmode=${PGSSLMODE})..."

psql -f /docker-entrypoint-initdb.d/init_seed_data.sql

echo "Database initialization completed successfully."
