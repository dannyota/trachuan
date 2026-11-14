#!/usr/bin/env bash
set -euo pipefail

dry_run=false
if [[ ${1:-} == --dry-run ]]; then
  dry_run=true
  shift
fi
if [[ $# != 4 ]]; then
  echo 'usage: corpus-upload.sh [--dry-run] <cc> <rel> <build-dir> <report.json>' >&2
  exit 2
fi
country=$1
rel=$2
build_dir=$3
report=$4
[[ $country =~ ^[a-z]{2}$ && $rel =~ ^[0-9]{8}_[0-9]+$ ]] || {
  echo 'Invalid corpus release identity' >&2; exit 2;
}
endpoint=${OCI_S3_ENDPOINT:?OCI_S3_ENDPOINT is required}
[[ $endpoint == https://* && $endpoint != *'@'* ]] || {
  echo 'OCI_S3_ENDPOINT must be HTTPS without credentials' >&2; exit 2;
}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export PYTHONDONTWRITEBYTECODE=1
if [[ $dry_run != true ]]; then
  mkdir -p "$root/work"
  exec 9> "$root/work/corpus-upload.lock"
  flock --exclusive --nonblock 9 || { echo 'Another corpus upload holds the local lock' >&2; exit 1; }
fi
python3 "$root/deploy/github/corpus_manifest.py" validate "$country" "$rel" "$build_dir" "$report"
prefix="$country/$rel/"
files=(corpus.dump MANIFEST.sha256 report.json release.json)

if [[ $dry_run == true ]]; then
  printf 'Check empty prefix s3://trachuan-releases/%s with profile trachuan-release-upload\n' "$prefix"
  for file in "${files[@]}"; do
    printf 'Upload and verify size and ETag: s3://trachuan-releases/%s%s\n' "$prefix" "$file"
  done
  printf 'gh workflow run corpus-release.yml --ref main -f country=%s -f rel=%s\n' "$country" "$rel"
  exit 0
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

python3 "$root/deploy/github/corpus_upload.py" "$country" "$rel" "$build_dir" "$report"
gh workflow run corpus-release.yml --ref main -f "country=$country" -f "rel=$rel"
