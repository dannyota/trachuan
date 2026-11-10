package chinhphu

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

func attr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func walk(n *xhtml.Node, f func(*xhtml.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}
func content(n *xhtml.Node) string {
	var b strings.Builder
	walk(n, func(v *xhtml.Node) {
		if v.Type == xhtml.TextNode {
			b.WriteString(v.Data)
		}
	})
	return b.String()
}
func hasClass(n *xhtml.Node, s string) bool {
	for _, v := range strings.Fields(attr(n, "class")) {
		if v == s {
			return true
		}
	}
	return false
}
func parseHTML(b []byte) (*xhtml.Node, error) {
	n, e := xhtml.Parse(bytes.NewReader(b))
	if e != nil {
		return nil, fmt.Errorf("parse HTML: %w", e)
	}
	return n, nil
}
func (c *Client) remember(value, name string) {
	if value == "" {
		return
	}
	c.secretsMu.Lock()
	defer c.secretsMu.Unlock()
	c.secrets[value] = "REDACTED-" + name
}
func (c *Client) form(n *xhtml.Node) url.Values {
	v := url.Values{}
	walk(n, func(x *xhtml.Node) {
		if x.Type != xhtml.ElementNode {
			return
		}
		name := attr(x, "name")
		if name == "" {
			return
		}
		switch x.Data {
		case "input":
			kind := strings.ToLower(attr(x, "type"))
			if kind == "submit" || kind == "image" || kind == "button" || kind == "checkbox" || kind == "radio" {
				return
			}
			val := attr(x, "value")
			if strings.HasPrefix(name, "__VIEWSTATE") || name == "__EVENTVALIDATION" {
				c.remember(val, name)
			}
			v.Set(name, val)
		case "select":
			value := "0"
			walk(x, func(opt *xhtml.Node) {
				if opt.Type != xhtml.ElementNode || opt.Data != "option" {
					return
				}
				for _, a := range opt.Attr {
					if a.Key == "selected" {
						value = attr(opt, "value")
						return
					}
				}
			})
			v.Set(name, value)
		}
	})
	return v
}

func (c *Client) redact(b []byte) []byte {
	c.secretsMu.Lock()
	defer c.secretsMu.Unlock()
	s := string(b)
	for value, replacement := range c.secrets {
		s = strings.ReplaceAll(s, html.EscapeString(value), replacement)
		s = strings.ReplaceAll(s, value, replacement)
	}
	return []byte(s)
}
func (c *Client) readPage(ctxReader io.Reader, header http.Header) ([]byte, error) {
	media := strings.ToLower(header.Get("Content-Type"))
	if media != "" && !strings.Contains(media, "text/html") && !strings.Contains(media, "application/xhtml") && !strings.Contains(media, "text/plain") {
		return nil, fmt.Errorf("invalid HTML media type: %w", errInvalidPayload)
	}
	b, e := io.ReadAll(io.LimitReader(ctxReader, c.limits.HTMLBytes+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > c.limits.HTMLBytes {
		return nil, ErrTooLarge
	}
	return b, nil
}

func (c *Client) rememberEncodedTokens(b []byte) {
	tokenizer := xhtml.NewTokenizer(bytes.NewReader(b))
	valuePattern := regexp.MustCompile(`(?i)(?:^|\s)value\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	for {
		typ := tokenizer.Next()
		if typ == xhtml.ErrorToken {
			return
		}
		if typ != xhtml.StartTagToken && typ != xhtml.SelfClosingTagToken {
			continue
		}
		raw := string(tokenizer.Raw())
		tok := tokenizer.Token()
		if tok.Data != "input" {
			continue
		}
		name := ""
		for _, a := range tok.Attr {
			if a.Key == "name" {
				name = a.Val
			}
		}
		if !strings.HasPrefix(name, "__VIEWSTATE") && name != "__EVENTVALIDATION" {
			continue
		}
		match := valuePattern.FindStringSubmatch(raw)
		if match == nil {
			continue
		}
		for _, v := range match[1:] {
			if v != "" {
				c.remember(v, name)
				break
			}
		}
	}
}

func isChallenge(n *xhtml.Node, b []byte) bool {
	form := false
	walk(n, func(x *xhtml.Node) {
		if x.Type == xhtml.ElementNode && x.Data == "form" {
			form = true
		}
	})
	lower := strings.ToLower(string(b))
	return !form && (strings.Contains(lower, "captcha") || strings.Contains(lower, "cf-chl") || strings.Contains(lower, "challenge-platform"))
}
