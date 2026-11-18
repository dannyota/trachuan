#!/usr/bin/env bash
set +x
set -euo pipefail
export ACTIVATOR_DB_PASSWORD
ACTIVATOR_DB_PASSWORD=$(cat /run/secrets/activator_db_password)
[[ -n $ACTIVATOR_DB_PASSWORD ]]
PGOPTIONS='-c log_statement=none -c log_min_error_statement=panic -c password_encryption=scram-sha-256 -c search_path=pg_catalog' \
/usr/local/bin/postgres-admin.sh psql -X -q --set ON_ERROR_STOP=1 --username postgres --dbname auth <<'SQL'
\getenv activator_password ACTIVATOR_DB_PASSWORD
SELECT 'CREATE ROLE activator LOGIN CREATEDB NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS'
WHERE NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname OPERATOR(pg_catalog.=) 'activator') \gexec
SELECT pg_catalog.format('ALTER ROLE activator PASSWORD %L', :'activator_password') \gexec
ALTER ROLE activator LOGIN CREATEDB NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE activator RESET ALL;
SELECT pg_catalog.format('ALTER ROLE activator IN DATABASE %I RESET ALL', d.datname)
FROM pg_catalog.pg_db_role_setting s
JOIN pg_catalog.pg_database d ON d.oid OPERATOR(pg_catalog.=) s.setdatabase
WHERE s.setrole OPERATOR(pg_catalog.=) 'activator'::pg_catalog.regrole
AND d.datname OPERATOR(pg_catalog.!~) '^frameworks_[0-9]{8}(_[0-9]{1,2})?$' \gexec
GRANT CONNECT ON DATABASE auth TO activator;
GRANT USAGE ON SCHEMA public TO activator;
SELECT pg_catalog.format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO activator', name)
FROM pg_catalog.unnest(ARRAY['corpus_release','release_journal','retired_release']) AS name
WHERE pg_catalog.to_regclass(pg_catalog.format('public.%I', name)) IS NOT NULL \gexec
SELECT 'GRANT USAGE, SELECT ON SEQUENCE public.release_journal_id_seq TO activator'
WHERE pg_catalog.to_regclass('public.release_journal_id_seq') IS NOT NULL \gexec
SELECT 'GRANT SELECT, INSERT, UPDATE ON public.frameworks_release TO activator'
WHERE pg_catalog.to_regclass('public.frameworks_release') IS NOT NULL \gexec
SQL
unset ACTIVATOR_DB_PASSWORD
