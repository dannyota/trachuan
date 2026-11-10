package retrieval

const (
	// SnippetCap is the snippet limit in docs/design/budgets.md.
	SnippetCap = 600
	// CompactDefaultLimit is the compact search default in docs/design/budgets.md.
	CompactDefaultLimit = 20
	// FullDefaultLimit is the full search default in docs/design/budgets.md.
	FullDefaultLimit = 8
	// MaxLimit is the search maximum in docs/design/budgets.md.
	MaxLimit = 50
	// PointersPerDocument is the pointer limit in docs/design/budgets.md.
	PointersPerDocument = 5
	// ReadItemsMax is the read item limit in docs/design/budgets.md.
	ReadItemsMax = 8
	// ReadCallCap is the read call limit in docs/design/budgets.md.
	ReadCallCap = 40000
	// PartSize is the read part limit in docs/design/budgets.md.
	PartSize = 16000
	// RelationPageSize is the relation page limit in docs/design/budgets.md.
	RelationPageSize = 20
)
