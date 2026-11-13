package retrieval

import "time"

type SourceWithdrawal struct {
	Since         Date   `json:"since"`
	AcquiredByRel string `json:"acquired_by_rel"`
}

func withdrawnColumns(version int) string {
	if version >= 6 {
		return ",source_withdrawn_since,source_withdrawn_rel"
	}
	return ",NULL::date,NULL::text"
}

func sourceWithdrawal(version int, since *time.Time, rel *string) *SourceWithdrawal {
	if version < 6 || since == nil || rel == nil {
		return nil
	}
	return &SourceWithdrawal{Since: *dateValue(since), AcquiredByRel: *rel}
}
