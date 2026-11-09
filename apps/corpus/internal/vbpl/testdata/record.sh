#!/bin/sh
set -eu

base=https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public
cd "$(dirname "$0")"
fetch() {
  output=$1
  shift
  curl --fail --silent --show-error \
    -H 'Origin: https://vbpl.vn' \
    -H 'Referer: https://vbpl.vn/' \
    -H 'User-Agent: trachuan/0.0 (+https://github.com/dannyota/trachuan)' \
    "$@" > "$output"
  sleep 1
}
fetch list-agencies-62-908-page1.json -H 'Content-Type: application/json' \
  --data '{"pageNumber":1,"pageSize":5,"sortBy":"issueDate","sortDirection":"desc","groupVbpl":false,"agencyLevel":"TRUNG_UONG","optionDoc":"title","matchMode":"all_words","agencyIds":["62","908"]}' "$base/doc/all"
for id in 67878 245a9a00-d447-11f1-84e4-cfbf13844ed1 66518; do
  fetch "detail-$id.json" "$base/doc/$id"
  fetch "tree-$id.json" "$base/doc/provision/tree/$id"
  fetch "diagram-$id.json" "$base/doc/$id/diagram"
  fetch "files-$id.json" "$base/doc/minio/buckets/vbpl/folders/$id/files"
  python3 - "files-$id.json" <<'PY'
import json
import sys
from urllib.parse import urlsplit, urlunsplit

path = sys.argv[1]
with open(path, encoding="utf-8") as source:
    response = json.load(source)
for item in response["data"]:
    url = item.get("presignedUrl")
    if url:
        parts = urlsplit(url)
        item["presignedUrl"] = urlunsplit((parts.scheme, parts.netloc, parts.path, "", ""))
with open(path, "w", encoding="utf-8") as target:
    json.dump(response, target, ensure_ascii=False, indent=2)
    target.write("\n")
PY
done
fetch detail-171687.json "$base/doc/171687"
fetch diagram-171687.json "$base/doc/171687/diagram"
