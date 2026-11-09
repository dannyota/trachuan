#!/usr/bin/env bash
# Create the pipeline, builder and Temporal roles and databases in the local compose
# PostgreSQL. Idempotent: reruns only reset the role passwords from .env.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
set -a
# shellcheck disable=SC1091
. ./.env
set +a
: "${PIPELINE_PASSWORD:?set PIPELINE_PASSWORD in .env}"
: "${TEMPORAL_DB_PASSWORD:?set TEMPORAL_DB_PASSWORD in .env}"
: "${BUILDER_PASSWORD:?set BUILDER_PASSWORD in .env}"
container="${CORPUS_PG_CONTAINER:-trachuan_postgres_1}"
psql_admin() {
  podman exec -i -e PIPELINE_PASSWORD -e TEMPORAL_DB_PASSWORD -e BUILDER_PASSWORD "$container" \
    psql -v ON_ERROR_STOP=1 -q -U trachuan -d postgres
}
psql_admin <<'SQL'
\getenv pipeline_pw PIPELINE_PASSWORD
\getenv temporal_pw TEMPORAL_DB_PASSWORD
\getenv builder_pw BUILDER_PASSWORD
SELECT 'CREATE ROLE pipeline LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pipeline') \gexec
SELECT 'CREATE ROLE corpus_builder LOGIN CREATEDB NOSUPERUSER NOCREATEROLE' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'corpus_builder') \gexec
SELECT 'CREATE ROLE temporal LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'temporal') \gexec
ALTER ROLE pipeline PASSWORD :'pipeline_pw';
ALTER ROLE temporal PASSWORD :'temporal_pw';
ALTER ROLE corpus_builder LOGIN CREATEDB NOSUPERUSER NOCREATEROLE PASSWORD :'builder_pw';
SELECT 'CREATE DATABASE corpus_template OWNER corpus_builder' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'corpus_template') \gexec
SELECT 'CREATE DATABASE workshop OWNER pipeline' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'workshop') \gexec
SELECT 'CREATE DATABASE temporal OWNER temporal' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'temporal') \gexec
SELECT 'CREATE DATABASE temporal_visibility OWNER temporal' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'temporal_visibility') \gexec
\connect corpus_template
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS ltree;
\connect postgres
ALTER DATABASE corpus_template IS_TEMPLATE true;
SQL
echo "db-setup: ok"
