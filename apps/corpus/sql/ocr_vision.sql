-- name: ListVisionSelectionKeys :many
SELECT item_key FROM run_ocr_selection
WHERE run_id = $1 AND gap = 'needs_ocr'
ORDER BY item_key;
