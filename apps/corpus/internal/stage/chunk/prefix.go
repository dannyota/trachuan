package chunk

import (
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk/normalizer"
)

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func capped(s string, limit int) string {
	r := []rune(collapse(s))
	if len(r) > limit {
		r = r[:limit]
	}
	return string(r)
}

func civilDate(s string) string {
	date, err := normalizer.ParseDate(s)
	if err != nil || date == nil {
		return ""
	}
	return date.Format("2006-01-02")
}

func prefix(meta Meta, owner *treeNode, recipe Recipe) string {
	fields := []string{capped(meta.Number, recipe.PrefixNumberLimit), capped(meta.Title, recipe.PrefixFieldLimit)}
	var chapter, section string
	for n := owner.parent; n != nil; n = n.parent {
		if n.node.Kind == "chuong" && chapter == "" {
			chapter = capped(n.node.Label+" "+n.node.Heading, recipe.PrefixFieldLimit)
		}
		if n.node.Kind == "muc" && section == "" {
			section = capped(n.node.Label+" "+n.node.Heading, recipe.PrefixFieldLimit)
		}
	}
	fields = append(fields, chapter, section)
	if date := civilDate(meta.EffFrom); date != "" {
		fields = append(fields, "Hiệu lực: "+date)
	} else if date := civilDate(meta.IssueDate); date != "" {
		fields = append(fields, "Ban hành: "+date)
	}
	out := fields[:0]
	for _, field := range fields {
		if field != "" {
			out = append(out, field)
		}
	}
	return strings.Join(out, "\n")
}
