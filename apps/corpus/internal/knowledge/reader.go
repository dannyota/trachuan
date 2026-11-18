package knowledge

import "context"

type Document struct {
	ID                      int64
	Key, Number, NumberNorm string
	TextSource              string
	TextComplete            bool
}

type Provision struct {
	ID                      int64
	ParentID                *int64
	NodeID, Anchor, Heading string
}

type RelationRow struct {
	Kind, Method                       string
	SourceDocumentID, TargetDocumentID int64
	SourceProvisionID                  *int64
	TargetProvisionID                  *int64
	TargetNodeID                       *string
}

type Reader interface {
	Documents(ctx context.Context, keys []string) ([]Document, error)
	BaselineServed(ctx context.Context, keys []string) ([]string, error)
	RelationRows(ctx context.Context, sourceDocumentIDs []int64) ([]RelationRow, error)
	Provisions(ctx context.Context, documentID int64) ([]Provision, error)
	ProvisionText(ctx context.Context, provisionID int64) (string, error)
}
