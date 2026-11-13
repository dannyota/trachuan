-- name: InsertSourceRequests :exec
INSERT INTO run_source_request (run_id, stage, source, endpoint, attempt, started_at, duration_ms, status, outcome)
SELECT sqlc.arg(run_id)::bigint, sqlc.arg(stage)::text, source, endpoint, attempt,
       started_at, duration_ms, NULLIF(status, 0), outcome
FROM (
    SELECT unnest(sqlc.arg(sources)::text[]) AS source,
           unnest(sqlc.arg(endpoints)::text[]) AS endpoint,
           unnest(sqlc.arg(attempts)::int[]) AS attempt,
           unnest(sqlc.arg(started_ats)::timestamptz[]) AS started_at,
           unnest(sqlc.arg(durations_ms)::bigint[]) AS duration_ms,
           unnest(sqlc.arg(statuses)::int[]) AS status,
           unnest(sqlc.arg(outcomes)::text[]) AS outcome
) AS requests;
