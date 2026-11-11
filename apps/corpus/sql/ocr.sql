-- name: InsertOCRText :one
INSERT INTO ocr_text (pdf_sha256, engine_id, text_sha256, bytes, origin, pages)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (pdf_sha256, engine_id) DO NOTHING
RETURNING id;

-- name: GetOCRText :one
SELECT id, pdf_sha256, engine_id, text_sha256, bytes, origin, pages, created_at
FROM ocr_text WHERE pdf_sha256 = $1 AND engine_id = $2;

-- name: ListOCRTextByPDF :many
SELECT id, pdf_sha256, engine_id, text_sha256, bytes, origin, pages, created_at
FROM ocr_text WHERE pdf_sha256 = ANY(sqlc.arg(pdf_sha256s)::text[])
ORDER BY pdf_sha256, engine_id;

-- name: InsertOCRCheck :one
INSERT INTO ocr_check (ocr_text_id, checker_id, result_sha256, result, flagged)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (ocr_text_id, checker_id) DO NOTHING
RETURNING id;

-- name: GetOCRCheck :one
SELECT id, ocr_text_id, checker_id, result_sha256, result, flagged, created_at
FROM ocr_check WHERE ocr_text_id = $1 AND checker_id = $2;

-- name: InsertOCRCheckAttempt :one
INSERT INTO ocr_check_attempt (ocr_text_id, checker_id, mode, submission_id, state)
VALUES ($1, $2, $3, $4, 'preparing')
RETURNING id;

-- name: MarkOCRCheckAttemptsSubmitted :execrows
UPDATE ocr_check_attempt
SET state = 'submitted', batch_id = sqlc.narg(batch_id), updated_at = now()
WHERE submission_id = sqlc.arg(submission_id) AND state IN ('preparing', 'uncertain');

-- name: MarkOCRCheckAttemptsUncertain :execrows
UPDATE ocr_check_attempt
SET state = 'uncertain', reason = sqlc.arg(reason), updated_at = now()
WHERE state = 'preparing'
  AND (submission_id = sqlc.narg(submission_id) OR (sqlc.narg(submission_id)::text IS NULL AND created_at < sqlc.arg(before)));

-- name: AbandonOCRCheckAttempts :execrows
UPDATE ocr_check_attempt
SET state = 'abandoned', updated_at = now()
WHERE submission_id = $1 AND state = 'uncertain';

-- name: FinishOCRCheckAttempt :execrows
UPDATE ocr_check_attempt
SET state = $2, reason = $3, usage = $4, updated_at = now()
WHERE id = $1 AND state IN ('preparing', 'submitted');

-- name: ListOpenOCRCheckAttempts :many
SELECT id, ocr_text_id, checker_id, mode, submission_id, batch_id, state, reason, usage, created_at, updated_at
FROM ocr_check_attempt
WHERE checker_id = $1 AND state IN ('preparing', 'submitted', 'uncertain')
ORDER BY id;

-- name: SumOCRCheckUsage :many
SELECT usage FROM ocr_check_attempt WHERE checker_id = $1 AND state <> 'submitted' ORDER BY id;

-- name: ListOCRChecks :many
SELECT c.id, c.ocr_text_id, c.checker_id, c.result_sha256, c.result, c.flagged, c.created_at, t.pdf_sha256, t.engine_id
FROM ocr_check c JOIN ocr_text t ON t.id = c.ocr_text_id
WHERE c.checker_id = $1 AND t.pdf_sha256 = ANY(sqlc.arg(pdf_sha256s)::text[])
ORDER BY t.pdf_sha256;

-- name: InsertRunOCRSelection :exec
INSERT INTO run_ocr_selection (run_id, item_key, gap, pdf_sha256, ocr_text_id, ocr_check_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (run_id, item_key) DO NOTHING;

-- name: GetRunOCRSelection :one
SELECT s.run_id, s.item_key, s.gap, s.pdf_sha256, s.ocr_text_id, s.ocr_check_id,
       t.engine_id, t.text_sha256, c.checker_id, c.result_sha256, c.flagged
FROM run_ocr_selection s
LEFT JOIN ocr_text t ON t.id = s.ocr_text_id
LEFT JOIN ocr_check c ON c.id = s.ocr_check_id
WHERE s.run_id = $1 AND s.item_key = $2;

-- name: ListRunOCRSelections :many
SELECT s.run_id, s.item_key, s.gap, s.pdf_sha256, s.ocr_text_id, s.ocr_check_id,
       t.engine_id, t.text_sha256, c.checker_id, c.result_sha256, c.flagged
FROM run_ocr_selection s
LEFT JOIN ocr_text t ON t.id = s.ocr_text_id
LEFT JOIN ocr_check c ON c.id = s.ocr_check_id
WHERE s.run_id = $1
ORDER BY s.item_key;
