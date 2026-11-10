#!/usr/bin/env bash
# Create the pipeline and builder roles, the workshop database and the corpus
# template in the local native PostgreSQL. Idempotent: reruns only reset the role
# passwords from .env.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
set -a
# shellcheck disable=SC1091
. ./.env
set +a
: "${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in .env}"
: "${PIPELINE_PASSWORD:?set PIPELINE_PASSWORD in .env}"
: "${BUILDER_PASSWORD:?set BUILDER_PASSWORD in .env}"
psql_admin() {
  PGPASSWORD="$POSTGRES_PASSWORD" "${TRACHUAN_PG_BIN:-/opt/homebrew/opt/postgresql@18/bin}/psql" \
    -v ON_ERROR_STOP=1 -q -h 127.0.0.1 -U trachuan -d postgres
}
psql_admin <<'SQL'
\getenv pipeline_pw PIPELINE_PASSWORD
\getenv builder_pw BUILDER_PASSWORD
SELECT 'CREATE ROLE pipeline LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pipeline') \gexec
SELECT 'CREATE ROLE corpus_builder LOGIN CREATEDB NOSUPERUSER NOCREATEROLE' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'corpus_builder') \gexec
ALTER ROLE pipeline PASSWORD :'pipeline_pw';
ALTER ROLE corpus_builder LOGIN CREATEDB NOSUPERUSER NOCREATEROLE PASSWORD :'builder_pw';
SELECT 'CREATE DATABASE corpus_template OWNER corpus_builder' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'corpus_template') \gexec
SELECT 'CREATE DATABASE workshop OWNER pipeline' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'workshop') \gexec
\connect corpus_template
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS ltree;
\connect postgres
ALTER DATABASE corpus_template IS_TEMPLATE true;
SQL
echo "db-setup: ok"
