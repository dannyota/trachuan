#!/usr/bin/env bash
set +x
set -euo pipefail
export ACTIVATOR_DB_PASSWORD
ACTIVATOR_DB_PASSWORD=$(cat /run/secrets/activator_db_password)
[[ -n $ACTIVATOR_DB_PASSWORD ]]
PGOPTIONS='-c log_statement=none -c log_min_error_statement=panic' \
/usr/local/bin/postgres-admin.sh psql -X -q --set ON_ERROR_STOP=1 --username postgres --dbname auth <<'SQL'
\getenv activator_password ACTIVATOR_DB_PASSWORD
SELECT 'CREATE ROLE activator LOGIN CREATEDB NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'activator') \gexec
SELECT format('ALTER ROLE activator PASSWORD %L', :'activator_password') \gexec
ALTER ROLE activator LOGIN CREATEDB NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS;
GRANT CONNECT ON DATABASE auth TO activator;
GRANT USAGE ON SCHEMA public TO activator;
SELECT format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO activator', name)
FROM unnest(ARRAY['corpus_release','release_journal','retired_release']) AS name
WHERE to_regclass('public.' || name) IS NOT NULL \gexec
SELECT 'GRANT USAGE, SELECT ON SEQUENCE release_journal_id_seq TO activator'
WHERE to_regclass('public.release_journal_id_seq') IS NOT NULL \gexec
SQL
unset ACTIVATOR_DB_PASSWORD
