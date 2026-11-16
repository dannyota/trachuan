package classify

import (
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type labelValidation struct {
	valid   map[string]bool
	invalid int
	total   int
}

func validateReleaseLabels(labels taxonomy.Labels, articles []Article, nodes []Node) (labelValidation, error) {
	v := labelValidation{valid: map[string]bool{}, total: len(labels.Articles)}
	if v.total == 0 {
		return v, fmt.Errorf("labeled set is empty")
	}
	texts := map[string]string{}
	for _, a := range articles {
		texts[a.DocumentKey+"/"+a.Anchor] = a.OwnText
	}
	active := map[string]bool{}
	for _, n := range nodes {
		active[n.Code] = n.Active
	}
	for _, label := range labels.Articles {
		key := label.Key + "/" + label.Anchor
		if _, seen := v.valid[key]; seen || label.Key == "" || label.Anchor == "" || len(label.Codes) == 0 {
			return v, fmt.Errorf("invalid or duplicate labeled article")
		}
		text, present := texts[key]
		valid := present && taxonomy.OwnTextSHA256(text) == label.OwnTextSHA256
		for _, code := range label.Codes {
			valid = valid && active[code]
		}
		v.valid[key] = valid
		if !valid {
			v.invalid++
		}
	}
	return v, nil
}

func (v labelValidation) availabilityError() error {
	if v.invalid*20 > v.total {
		return fmt.Errorf("more than 5 percent of labeled entries fail release validation")
	}
	return nil
}

func ValidateScreenSelection(labels taxonomy.Labels, articles []Article, nodes []Node) error {
	v, err := validateReleaseLabels(labels, articles, nodes)
	if err != nil {
		return err
	}
	return v.availabilityError()
}
