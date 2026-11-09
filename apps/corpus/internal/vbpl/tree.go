package vbpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrTreeTooLarge means a provision tree exceeds its depth or node limit.
var ErrTreeTooLarge = errors.New("vbpl: tree too large")

const (
	// MaxTreeDepth is the provision tree depth budget.
	MaxTreeDepth = 32
	// MaxTreeNodes is the provision tree node budget.
	MaxTreeNodes = 50000
)

type treeDecoder struct {
	decoder      *json.Decoder
	ctx          context.Context
	depth, nodes int
	count        int
}

// DecodeTree reads a gateway envelope and bounds its provision tree while decoding.
func DecodeTree(ctx context.Context, r io.Reader, maxDepth, maxNodes int) ([]Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}
	d := treeDecoder{decoder: json.NewDecoder(r), ctx: ctx, depth: maxDepth, nodes: maxNodes}
	nodes, err := d.envelope()
	if err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}
	if _, trailingErr := d.decoder.Token(); trailingErr != io.EOF {
		if trailingErr == nil {
			return nil, fmt.Errorf("decode tree: trailing JSON value")
		}
		return nil, fmt.Errorf("decode tree: %w", trailingErr)
	}
	return nodes, nil
}

func (d *treeDecoder) delim(want json.Delim) error {
	token, err := d.decoder.Token()
	if err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("expected %q, got %v", want, token)
	}
	return nil
}

func (d *treeDecoder) envelope() ([]Node, error) {
	if err := d.delim('{'); err != nil {
		return nil, err
	}
	var nodes []Node
	var success bool
	for d.decoder.More() {
		key, err := d.decoder.Token()
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(key.(string)) {
		case "success":
			if decodeErr := d.decoder.Decode(&success); decodeErr != nil {
				return nil, decodeErr
			}
		case "data":
			nodes, err = d.array(1)
			if err != nil {
				return nil, err
			}
		default:
			if skipErr := d.skip(); skipErr != nil {
				return nil, skipErr
			}
		}
	}
	if err := d.delim('}'); err != nil {
		return nil, err
	}
	if !success {
		return nil, ErrNotFound
	}
	return nodes, nil
}

func (d *treeDecoder) array(depth int) ([]Node, error) {
	token, err := d.decoder.Token()
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, nil
	}
	if token != json.Delim('[') {
		return nil, fmt.Errorf("expected array, got %v", token)
	}
	var nodes []Node
	for d.decoder.More() {
		if depth > d.depth || d.nodes <= 0 {
			return nil, ErrTreeTooLarge
		}
		d.nodes--
		d.count++
		if d.count%1000 == 0 {
			if contextErr := d.ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
		}
		node, nodeErr := d.node(depth)
		if nodeErr != nil {
			return nil, nodeErr
		}
		nodes = append(nodes, node)
	}
	if delimErr := d.delim(']'); delimErr != nil {
		return nil, delimErr
	}
	return nodes, nil
}

func (d *treeDecoder) node(depth int) (Node, error) {
	var node Node
	if err := d.delim('{'); err != nil {
		return node, err
	}
	for d.decoder.More() {
		key, err := d.decoder.Token()
		if err != nil {
			return node, err
		}
		switch strings.ToLower(key.(string)) {
		case "id":
			err = d.decoder.Decode(&node.ID)
		case "title":
			err = d.decoder.Decode(&node.Title)
		case "level":
			err = d.decoder.Decode(&node.Level)
		case "ptype":
			err = d.decoder.Decode(&node.PType)
		case "orderindex":
			err = d.decoder.Decode(&node.OrderIndex)
		case "content":
			var content *struct {
				Content string `json:"content"`
			}
			err = d.decoder.Decode(&content)
			if content != nil {
				node.Content = content.Content
			}
		case "children":
			node.Children, err = d.array(depth + 1)
		default:
			err = d.skip()
		}
		if err != nil {
			return node, err
		}
	}
	if delimErr := d.delim('}'); delimErr != nil {
		return node, delimErr
	}
	return node, nil
}

func (d *treeDecoder) skip() error {
	token, err := d.decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || (delim != '{' && delim != '[') {
		return nil
	}
	depth := 1
	for depth > 0 {
		token, err = d.decoder.Token()
		if err != nil {
			return err
		}
		if delim, ok = token.(json.Delim); ok {
			switch delim {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return nil
}
