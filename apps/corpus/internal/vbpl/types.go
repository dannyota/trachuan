// Package vbpl reads the Vietnamese legal database gateway.
package vbpl

import "encoding/json"

// ListQuery identifies a page of a gateway listing.
type ListQuery struct {
	AgencyIDs []string
	Keyword   string
	Page      int
}

// ListPage holds a listing page and its total size.
type ListPage struct {
	Total int
	Items []ListItem
}

// ListItem holds listing metadata and the original item.
type ListItem struct {
	ID, DocNum, Title, IssueDate string
	Raw                          json.RawMessage
}

// Detail holds document metadata.
type Detail struct {
	ID, DocNum, Title, DocType, Agency, IssueDate, EffFrom, EffTo, StatusCode, StatusName string
	IsConsolidated, HasContent                                                            bool
	References                                                                            []Reference
}

// Reference holds a source relation and its original representation.
type Reference struct {
	TargetID, TargetNumber, Code, Label string
	Raw                                 json.RawMessage
}

// Node holds a provision and its children.
type Node struct {
	ID, Title, Level, Content string
	PType, OrderIndex         int
	Children                  []Node
}

// UnmarshalJSON decodes the gateway's node shape, whose content is an object
// holding the node's own title and HTML text, or null.
func (n *Node) UnmarshalJSON(b []byte) error {
	var wire struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Level      string `json:"level"`
		PType      int    `json:"ptype"`
		OrderIndex int    `json:"orderIndex"`
		Content    *struct {
			Content string `json:"content"`
		} `json:"content"`
		Children []Node `json:"children"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}
	*n = Node{ID: wire.ID, Title: wire.Title, Level: wire.Level, PType: wire.PType, OrderIndex: wire.OrderIndex, Children: wire.Children}
	if wire.Content != nil {
		n.Content = wire.Content.Content
	}
	return nil
}

// File identifies an attachment in the gateway listing.
type File struct {
	Name, FullPath string
	Size           int64
	URL            string
}

// Downloaded identifies a completed local attachment.
type Downloaded struct {
	Path, SHA256 string
	Size         int64
}
