-- name: ListDiscoveryRuns :many
SELECT id, rel, snapshot FROM run WHERE country = $1;

-- name: ListDiscoveryListings :many
SELECT listing.run_id, listing.query_key, listing.item_key
FROM listing JOIN run ON run.id = listing.run_id
WHERE run.country = $1;

-- name: ListDiscoveryItems :many
SELECT run_item.run_id, run_item.item_key,
       (run_item.state = 'done' AND NOT EXISTS (
           SELECT 1 FROM gap WHERE gap.run_id = run_item.run_id
             AND gap.stage = 'discover' AND gap.item_key = run_item.item_key
             AND gap.kind = 'listing_unstable'
       ))::boolean AS complete
FROM run_item JOIN run ON run.id = run_item.run_id
WHERE run.country = $1 AND run_item.stage = 'discover';
