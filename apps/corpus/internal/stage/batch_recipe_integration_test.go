//go:build integration

package stage

import "testing"

func TestRunBatch_RefusesOldPendingRecipeWithoutCommitting(t *testing.T) {
	env, id, s := batchSetup(t)
	batchExec(t, env.DB, `UPDATE run SET snapshot='{"Recipes":{"fetch":"old"}}' WHERE id=$1`, id)
	result, err := batchRun(env, id, s, 1, func(...any) { t.Error("heartbeat started before recipe validation") })
	state, gaps := batchState(t, env.DB, id)
	if err == nil || err.Error() != "recipe mismatch for stage fetch" || result != (BatchResult{}) || s.calls != 0 || state != "pending" || gaps != 0 {
		t.Fatalf("result=%+v err=%v calls=%d state=%s gaps=%d", result, err, s.calls, state, gaps)
	}
	var artifacts, attempt int
	if queryErr := env.DB.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM artifact),attempt FROM run_item WHERE run_id=$1 AND item_key='item'`, id).Scan(&artifacts, &attempt); queryErr != nil || artifacts != 0 || attempt != 0 {
		t.Fatalf("artifacts=%d attempt=%d err=%v", artifacts, attempt, queryErr)
	}
}
