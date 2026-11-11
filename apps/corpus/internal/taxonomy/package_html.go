package taxonomy

import (
	"html"
	"regexp"
	"strings"
)

func appendArticles(out []Article, data []byte, section string, tree map[string]string) []Article {
	article := regexp.MustCompile(`(?s)<p class='pDieu'><a name='([^']+)'></a>.*?</p>(?:\s*<p class='pGhiChu'>(.*?)</p>)?`)
	link := regexp.MustCompile(`(?s)<a\s+[^>]*href='([^']*)'`)
	item := regexp.MustCompile(`(?:[?&])ItemID=([0-9]+)(?:[&#]|$)`)
	anchor := regexp.MustCompile(`(?:^|#)Dieu_([0-9]+)(?:$|[^0-9A-Za-z_])`)
	noteNumber := regexp.MustCompile(`\(Điều\s+([0-9]+)(?:[^0-9A-Za-z_]|$)`)
	tags := regexp.MustCompile(`<[^>]*>`)
	for _, m := range article.FindAllSubmatch(data, -1) {
		a := Article{DeMucGUID: section, MAPC: string(m[1]), Seq: len(out) + 1, TreeGUID: tree[string(m[1])]}
		note := string(m[2])
		for _, href := range link.FindAllStringSubmatch(note, -1) {
			url := html.UnescapeString(href[1])
			if !strings.Contains(url, "vbpq-toanvan.aspx") {
				continue
			}
			x := item.FindStringSubmatch(url)
			if len(x) < 2 {
				continue
			}
			a.ItemID = x[1]
			if x := anchor.FindStringSubmatch(url); len(x) > 1 {
				a.Number = x[1]
			}
			break
		}
		if a.Number == "" {
			txt := html.UnescapeString(tags.ReplaceAllString(note, " "))
			if x := noteNumber.FindStringSubmatch(txt); len(x) > 1 {
				a.Number = x[1]
			}
		}
		out = append(out, a)
	}
	return out
}
