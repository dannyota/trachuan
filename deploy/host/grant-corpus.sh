#!/usr/bin/env bash
# Run inside PostgreSQL after restoring a release with --no-owner as postgres.
set -euo pipefail
database=${1:?usage: grant-corpus.sh corpus_database}
[[ $database =~ ^corpus_[a-z]{2}_[0-9]{8}(_[0-9]+)?$ ]] || { echo 'invalid corpus database name' >&2; exit 1; }
/usr/local/bin/postgres-admin.sh psql -X -q --set ON_ERROR_STOP=1 --username postgres --dbname postgres --set database="$database" <<'SQL'
REVOKE ALL ON DATABASE :"database" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"database" TO corpus_reader;
SQL
/usr/local/bin/postgres-admin.sh psql -X -q --set ON_ERROR_STOP=1 --username postgres --dbname "$database" <<'SQL'
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO corpus_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO corpus_reader;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO corpus_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA public GRANT SELECT ON TABLES TO corpus_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA public GRANT SELECT ON SEQUENCES TO corpus_reader;
SQL
