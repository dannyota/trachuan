package fetch

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

func checkDetailIdentity(body []byte, id string) error {
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	for {
		typ := tokenizer.Next()
		if typ == html.ErrorToken {
			return nil
		}
		if typ != html.StartTagToken && typ != html.SelfClosingTagToken {
			continue
		}
		tok := tokenizer.Token()
		if tok.Data != "meta" {
			continue
		}
		property, content := "", ""
		for _, a := range tok.Attr {
			if a.Key == "property" {
				property = a.Val
			}
			if a.Key == "content" {
				content = a.Val
			}
		}
		if !strings.EqualFold(property, "og:url") {
			continue
		}
		u, err := url.Parse(content)
		if err != nil {
			return fmt.Errorf("invalid detail identity URL: %w", err)
		}
		if got := u.Query().Get("docid"); got != "" && got != id {
			return fmt.Errorf("detail identity disagrees with requested document")
		}
	}
}
