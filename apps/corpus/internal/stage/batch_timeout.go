package stage

// repeatedSourceTimeout ends a source item after its second batch timeout.
func repeatedSourceTimeout(name string, attempt int, err *RetryableError) bool {
	return (name == "fetch" || name == "expand") && attempt >= 2 &&
		err.GapKind == "source_unavailable" && err.Cause.Cause == CauseTimeout
}
