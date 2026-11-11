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
