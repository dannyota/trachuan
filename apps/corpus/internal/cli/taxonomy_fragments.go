package cli

import (
	"context"
	"fmt"
	"io"

	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func runTaxonomyFragmentsWith(ctx context.Context, args []string, stdout, stderr io.Writer, load func(context.Context, string, string) (drawEvidence, error)) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if len(args) != 4 || args[0] != "--labels" || args[1] == "" {
		return fail(2, fmt.Errorf("usage: corpus taxonomy fragments --labels FILE <cc> <rel>"))
	}
	cc, rel := args[2], args[3]
	if _, err := build.DatabaseName(cc, rel); err != nil {
		return fail(2, err)
	}
	var labels taxonomy.Labels
	if err := readTaxonomyJSON(args[1], &labels); err != nil {
		return fail(1, err)
	}
	if labels.Country != cc || labels.Release != rel || len(labels.Articles) != 300 {
		return fail(1, fmt.Errorf("invalid frozen seeded set identity or size"))
	}
	e, err := load(ctx, cc, rel)
	if err != nil {
		return fail(1, err)
	}
	if e.manualCount != 0 {
		return fail(1, fmt.Errorf("release has manual assignments"))
	}
	byID := make(map[string]taxonomy.PacketArticle, len(e.articles))
	for _, a := range e.articles {
		byID[a.Key+"\x00"+a.Anchor] = a.PacketArticle
	}
	seen := map[string]bool{}
	count := 0
	for _, a := range labels.Articles {
		id := a.Key + "\x00" + a.Anchor
		text, ok := byID[id]
		if !ok || seen[id] || taxonomy.OwnTextSHA256(text.Text) != a.OwnTextSHA256 {
			return fail(1, fmt.Errorf("missing, duplicate, or changed article %s %s", a.Key, a.Anchor))
		}
		seen[id] = true
		if taxonomy.IsFragment(text.Text) {
			count++
		}
	}
	_, _ = fmt.Fprintf(stdout, "fragments: %d of %d\n", count, len(labels.Articles))
	return 0
}
