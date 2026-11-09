// Package vbpl reads the Vietnamese legal database gateway.
package vbpl

import (
	"encoding/json"
	"time"
)

// ListQuery identifies a page of a gateway listing.
type ListQuery struct {
	AgencyIDs           []string
	Keyword             string
	Page                int
	PageSize            int
	Option, Mode, Level string
}

// ListPage holds a listing page and its total size.
type ListPage struct {
	Total      int
	Items      []ListItem
	Raw        json.RawMessage
	totalKnown bool
}

// ListItem holds listing metadata and the original item.
type ListItem struct {
	ID, DocNum, Title, IssueDate string
	Raw                          json.RawMessage
	AgencyIDs                    []string
	AgencyName                   string
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

// File identifies an attachment in the gateway listing.
type File struct {
	Name, FullPath string
	Size           int64
	URL            string
	UpdatedAt      time.Time
}

// Downloaded identifies a completed local attachment.
type Downloaded struct {
	Path, SHA256 string
	Size         int64
}
