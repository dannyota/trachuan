package classify

import "fmt"

func validateStepMenu(r Recipe, req ArticleRequest) error {
	if req.Step != r.Article.Step {
		return fmt.Errorf("request step differs from recipe")
	}
	if req.Step == 0 {
		return nil
	}
	codes := make([]string, 0, len(req.Nodes))
	for _, n := range req.Nodes {
		if !n.Active || (n.Origin != "phapdien" && n.Origin != "custom") || n.Level < 2 || n.Level > 4 {
			return fmt.Errorf("invalid step menu node")
		}
		if n.Level == 2 {
			codes = append(codes, n.Code)
		} else if req.Step == 1 {
			return fmt.Errorf("step 1 accepts level-2 nodes only")
		}
	}
	if req.Step == 1 {
		if req.Family != "" {
			return fmt.Errorf("step 1 cannot name a single family")
		}
		return nil
	}
	if len(codes) != 1 || codes[0] != req.Family {
		return fmt.Errorf("step 2 requires exactly one family subtree")
	}
	allowed, err := allowedNodes(codes, req.Nodes)
	if err != nil {
		return err
	}
	if len(allowed) != len(req.Nodes) {
		return fmt.Errorf("step 2 menu includes nodes outside its family")
	}
	return nil
}
