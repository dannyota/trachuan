#!/usr/bin/env bash
set -euo pipefail

dry_run=false
if [[ ${1:-} == --dry-run ]]; then
  dry_run=true
  shift
fi
frameworks=false
if [[ $# == 2 && $1 == frameworks ]]; then
  frameworks=true
  rel=$2
  [[ $rel =~ ^[0-9]{8}(_[0-9]{1,2})?$ ]] || {
    echo 'Invalid frameworks release identity' >&2; exit 2;
  }
elif [[ $# != 4 ]]; then
  echo 'usage: corpus-upload.sh [--dry-run] <cc> <rel> <build-dir> <report.json>' >&2
  echo '       corpus-upload.sh [--dry-run] frameworks <rel>' >&2
  exit 2
else
  country=$1
  rel=$2
  build_dir=$3
  report=$4
  [[ $country =~ ^[a-z]{2}$ && $rel =~ ^[0-9]{8}_[0-9]+$ ]] || {
    echo 'Invalid corpus release identity' >&2; exit 2;
  }
fi
endpoint=${OCI_S3_ENDPOINT:?OCI_S3_ENDPOINT is required}
[[ $endpoint == https://* && $endpoint != *'@'* ]] || {
  echo 'OCI_S3_ENDPOINT must be HTTPS without credentials' >&2; exit 2;
}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export PYTHONDONTWRITEBYTECODE=1
if [[ $dry_run != true ]]; then
  umask 077
  lock_directory="${HOME:?HOME is required}/.cache/trachuan"
  mkdir -p "$lock_directory"
  chmod 0700 "$lock_directory"
  exec 9> "$lock_directory/corpus-upload.lock"
  flock --exclusive --nonblock 9 || { echo 'Another corpus upload holds the local lock' >&2; exit 1; }
fi
if [[ $frameworks == true ]]; then
  if [[ $dry_run == true ]]; then
    exec python3 "$root/deploy/github/frameworks_upload.py" --dry-run "$rel"
  fi
  exec python3 "$root/deploy/github/frameworks_upload.py" "$rel"
fi
files=$(python3 "$root/deploy/github/corpus_manifest.py" validate "$country" "$rel" "$build_dir" "$report" --list-files)
prefix="$country/$rel/"

if [[ $dry_run == true ]]; then
  printf 'Require report previous rel and SHA-256 to equal the verified %s-production manifest\n' "$country"
  printf 'Require release of https://trachuan.vn/api/v1/public/status to equal that rel\n'
  printf 'Verify previous signed manifest and report SHA-256; recompute comparisons before upload\n'
  printf 'Check empty prefix s3://trachuan-releases/%s with profile trachuan-release-upload\n' "$prefix"
  while IFS= read -r file; do
    printf 'Upload and verify size and ETag: s3://trachuan-releases/%s%s\n' "$prefix" "$file"
  done <<< "$files"
  exec python3 "$root/deploy/github/corpus_upload.py" --dry-run "$country" "$rel" "$build_dir" "$report"
fi

version=$(aws --version)
[[ $version == aws-cli/2.* ]] || { echo 'AWS CLI v2 is required' >&2; exit 1; }
printf '%s\n' "$version"
umask 077
directory=$(mktemp -d)
trap 'rm -rf "$directory"' EXIT
export AWS_CONFIG_FILE="$directory/config"
export AWS_REQUEST_CHECKSUM_CALCULATION=when_required
printf '%s\n' '[profile trachuan-release-upload]' 'region = ap-singapore-1' \
  's3 =' '    addressing_style = path' '    signature_version = s3v4' > "$AWS_CONFIG_FILE"
aws_release() {
  aws --profile trachuan-release-upload --endpoint-url "$endpoint" --no-cli-pager "$@"
}
count=$(aws_release s3api list-objects-v2 --bucket trachuan-releases --prefix "$prefix" \
  --max-keys 1 --query KeyCount --output text)
[[ $count == 0 ]] || { echo 'Release prefix already holds objects; choose a new rel' >&2; exit 1; }

cp "$report" "$directory/report.json"
python3 "$root/deploy/github/corpus_upload.py" "$country" "$rel" "$build_dir" "$directory/report.json"
