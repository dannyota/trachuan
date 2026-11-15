package heading

import "strings"

// TextOnly reports whether the text contains no more than the label and heading.
func TextOnly(kind, label, heading, text string) bool {
	text = collapse(text)
	label = collapse(label)
	heading = collapse(heading)
	if text == "" && kind != "dieu" {
		return true
	}
	if heading != "" && (text == label || text == label+".") {
		return false
	}
	return text == label || text == label+"." || text == label+" "+heading || text == label+". "+heading
}

// ArticleOnly also requires an article to have no child provisions.
func ArticleOnly(kind, label, heading, text string, hasChildren bool) bool {
	return kind == "dieu" && !hasChildren && TextOnly(kind, label, heading, text)
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
