#!/usr/bin/env bash
set +x
set +a
set -euo pipefail
umask 077
fail() { echo 'pgbackrest-entrypoint: missing, empty or invalid secret' >&2; exit 1; }
secrets=${CREDENTIALS_DIRECTORY:-/run/secrets}
include=${PGBACKREST_CONFIG_INCLUDE_PATH:-/run/pgbackrest/conf.d}
passfile=${PGPASSFILE:-/run/pgbackrest/pgpass}
repository_only=false
if [[ ${1:-} == --repository-only ]]; then
 repository_only=true
 shift
fi
unset backup_key backup_secret database_password PGPASSWORD
[[ -s $secrets/backup_s3_key && -r $secrets/backup_s3_key && -s $secrets/backup_s3_key_secret && -r $secrets/backup_s3_key_secret ]] || fail
backup_key=$(<"$secrets/backup_s3_key")
backup_secret=$(<"$secrets/backup_s3_key_secret")
for value in "$backup_key" "$backup_secret"; do
 [[ -n $value && $value != *$'\n'* && $value != *$'\r'* && $value != [[:space:]]* && $value != *[[:space:]] ]] || fail
done
if [[ $repository_only == false ]]; then
 [[ -s $secrets/postgres_password && -r $secrets/postgres_password ]] || fail
 database_password=$(<"$secrets/postgres_password")
 [[ -n $database_password && $database_password != *$'\n'* && $database_password != *$'\r'* ]] || fail
fi
mkdir -p "$include"
chmod 0700 "$(dirname "$include")" "$include"
printf '[global]\nrepo1-s3-key=%s\nrepo1-s3-key-secret=%s\n' "$backup_key" "$backup_secret" >"$include/secret.conf"
chmod 0600 "$include/secret.conf"
if [[ $repository_only == false ]]; then
 database_password=${database_password//\\/\\\\}
 database_password=${database_password//:/\\:}
 printf '*:*:*:postgres:%s\n' "$database_password" >"$passfile"
 chmod 0600 "$passfile"
fi
unset backup_key backup_secret database_password value
exec docker-entrypoint.sh "$@"
