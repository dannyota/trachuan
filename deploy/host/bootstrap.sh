#!/usr/bin/env bash
# Runs inside PostgreSQL on the first initialization of an empty data volume.
set -euo pipefail
export AUTH_DB_PASSWORD CORPUS_READER_PASSWORD
AUTH_DB_PASSWORD=$(cat /run/secrets/auth_db_password)
CORPUS_READER_PASSWORD=$(cat /run/secrets/corpus_reader_password)
# Keep password-bearing statements out of server error logs as well as psql output.
PGOPTIONS='-c log_statement=none -c log_min_error_statement=panic' \
/usr/local/bin/postgres-admin.sh psql -X -q --set ON_ERROR_STOP=1 --username postgres --dbname postgres <<'SQL'
\getenv auth_password AUTH_DB_PASSWORD
\getenv corpus_password CORPUS_READER_PASSWORD
SELECT 'CREATE ROLE auth_owner LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'auth_owner') \gexec
SELECT 'CREATE ROLE corpus_reader LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'corpus_reader') \gexec
SELECT format('ALTER ROLE auth_owner PASSWORD %L', :'auth_password') \gexec
SELECT format('ALTER ROLE corpus_reader PASSWORD %L', :'corpus_password') \gexec
ALTER ROLE auth_owner NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE corpus_reader NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE corpus_reader SET default_transaction_read_only = on;
SELECT 'CREATE DATABASE auth OWNER auth_owner' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'auth') \gexec
REVOKE ALL ON DATABASE auth FROM PUBLIC;
GRANT CONNECT ON DATABASE auth TO auth_owner;
\connect auth
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SQL
unset AUTH_DB_PASSWORD CORPUS_READER_PASSWORD
