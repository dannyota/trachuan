# Abstention calibration

This note owns the abstention threshold. [Evaluation](evaluation.md) owns the
cohorts, metrics and report.

Build calls the same engine on old product and development exact hybrid limit-8
responses, before finalizing the release row. No lexical, chunk or model
experiment runs. Cache first-group best-member cosines before applying a
threshold; empty or rejected cases remain fixed abstentions. Candidate
thresholds are midpoints between consecutive distinct observed cosines, plus
always-answer/always-abstain sentinels defined in budgets. Choose the threshold
that abstains on every negative of both cohorts while maximizing positive answer
rate; ties choose the lowest threshold. If none answer any positive, keep the
always-abstain threshold and report that failure rather than weaken the
condition.

Calibration calls `Prepare` and `Collect` on the draft, then `retrieval.Rank`,
which returns `FirstBestCosine` without a threshold; it never calls `Search`.
Hash only the pre-calibration retrieval policy in `calibration.json`, never the
finalized recipe that will contain that file's hash. Use finite float64 cosine
values clamped to [-1,1]; if a rounded midpoint equals the lower score, use the
upper score, since equality passes. Exclude the three rejection checks from
fitting.

Record threshold, exact float values, fixture/mapping/recipe hashes, date and
all candidate TP/FP/TN/FN counts in `calibration.json`. Hash that file into
`retrieval_recipe.abstention.calibration_sha256`. The chosen numeric threshold
is computed from real scores, not asserted by this design or fixed at the
historical Qwen seed. Build adopts the first published artifact on retries. Eval
never re-fits the threshold; ANN uses the same frozen value.

Until development cases exist, calibration uses old product alone and every
report labels it `provisional-old-product`; afterwards the label is
`old-product+development`. Fitting on the scored cohorts biases their abstention
result; it establishes neither independent validation nor a model improvement.
Later calibration changes the retrieval recipe and requires a new build.
