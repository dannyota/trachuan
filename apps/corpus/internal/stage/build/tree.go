package build

import (
	"fmt"
	"strconv"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

type treeNode struct {
	Node   chunk.Node
	Path   string
	Parent int
}

func preorder(nodes []chunk.Node) ([]treeNode, error) {
	result := make([]treeNode, 0, len(nodes))
	stack := []int{}
	ids := make(map[string]bool, len(nodes))
	anchors := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		if node.NodeID == "" || node.Path == "" || node.Ordinal <= 0 || ids[node.NodeID] || anchors[node.Path] {
			return nil, fmt.Errorf("invalid provision identity")
		}
		ids[node.NodeID] = true
		anchors[node.Path] = true
		parent := -1
		if node.ParentNodeID == nil {
			stack = stack[:0]
		} else {
			for len(stack) > 0 && result[stack[len(stack)-1]].Node.NodeID != *node.ParentNodeID {
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 {
				return nil, fmt.Errorf("invalid provision ancestry")
			}
			parent = stack[len(stack)-1]
		}
		path := "n" + strconv.Itoa(len(result)+1)
		if parent >= 0 {
			path = result[parent].Path + "." + path
		}
		result = append(result, treeNode{Node: node, Path: path, Parent: parent})
		stack = append(stack, len(result)-1)
	}
	return result, nil
}
