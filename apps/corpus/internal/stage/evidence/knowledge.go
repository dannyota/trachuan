package evidence

const knowledgeFile = "knowledge.yaml"

func reviewDecisions(knowledge []byte) ([]ReviewDecision, error) {
	if knowledge == nil {
		return nil, nil
	}
	return nil, &IncompleteError{Reason: "knowledge parser unavailable"}
}
