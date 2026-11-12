#!/bin/sh
# Records the corpus scope fixtures: exact number lookups, exact title listings
# and relation direction. It records what the gateway returns and stops only on
# transport failure or a non-2xx status.
set -eu

user_agent='Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36'
base=https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public
cd "$(dirname "$0")"
fetch() {
  output=$1
  shift
  curl --fail --silent --show-error \
    -H 'Origin: https://vbpl.vn' \
    -H 'Referer: https://vbpl.vn/' \
    -H "User-Agent: $user_agent" \
    "$@" > "$output"
  sleep 1
}
# lookup <output> <number> [page size] [page]
lookup() {
  fetch "$1" -H 'Content-Type: application/json' \
    --data "{\"pageNumber\":${4:-1},\"pageSize\":${3:-50},\"sortBy\":\"issueDate\",\"sortDirection\":\"desc\",\"groupVbpl\":false,\"agencyLevel\":\"TRUNG_UONG\",\"optionDoc\":\"docNum\",\"matchMode\":\"exact\",\"keyword\":\"$2\"}" \
    "$base/doc/all"
}
# title <output> <mode> <keyword>
title() {
  fetch "$1" -H 'Content-Type: application/json' \
    --data "{\"pageNumber\":1,\"pageSize\":50,\"sortBy\":\"issueDate\",\"sortDirection\":\"desc\",\"groupVbpl\":false,\"agencyLevel\":\"TRUNG_UONG\",\"optionDoc\":\"title\",\"matchMode\":\"$2\",\"agencyIds\":[\"55\",\"56\",\"1\",\"57\",\"3\",\"14\",\"169\",\"2\"],\"keyword\":\"$3\"}" \
    "$base/doc/all"
}
lookup lookup-71-2025-QH15.json '71/2025/QH15'
lookup lookup-367-QD-TTg.json '367/QĐ-TTg'
lookup lookup-142-2026-ND-CP.json '142/2026/NĐ-CP'
lookup lookup-367-QD-TTg-size2-page1.json '367/QĐ-TTg' 2 1
lookup lookup-367-QD-TTg-size2-page2.json '367/QĐ-TTg' 2 2
lookup lookup-367-QD-TTg-size2-page3.json '367/QĐ-TTg' 2 3
# The same number with a Cyrillic Er (U+0420) in place of the Latin P.
lookup lookup-368-2025-ND-CP-cyrillic.json '368/2025/NĐ-CР'
title list-exact-tai-san-so.json exact 'tài sản số'
title list-all-words-thanh-toan-khong-dung-tien-mat.json all_words 'thanh toán không dùng tiền mặt'
title list-exact-thanh-toan-khong-dung-tien-mat.json exact 'thanh toán không dùng tiền mặt'
# 171687 replaces 117031 (code 12); 186058 amends 171687 (code 10).
fetch diagram-117031.json "$base/doc/117031/diagram"
fetch diagram-186058.json "$base/doc/186058/diagram"
# The full central listing: no keyword and no agency, page size 5.
for page in 1 2; do
  fetch "list-central-size5-page$page.json" -H 'Content-Type: application/json' \
    --data "{\"pageNumber\":$page,\"pageSize\":5,\"sortBy\":\"issueDate\",\"sortDirection\":\"desc\",\"groupVbpl\":false,\"agencyLevel\":\"TRUNG_UONG\",\"optionDoc\":\"title\",\"matchMode\":\"all_words\"}" \
    "$base/doc/all"
done
