package evidence

import (
	"context"
	"slices"
	"strings"
)

func resolveClaims(ctx context.Context, rel releaseData, cov *coverage, scan claimScan) (Observation, error) {
	wanted := map[int64][]string{}
	for _, c := range scan.claims {
		for _, t := range c.targets {
			if t.ok {
				wanted[t.document] = append(wanted[t.document], t.anchor)
			}
		}
	}
	documents := make([]int64, 0, len(wanted))
	for id := range wanted {
		documents = append(documents, id)
	}
	slices.Sort(documents)
	resolvable := map[int64]map[string]bool{}
	for _, id := range documents {
		anchors, err := rel.Anchors(ctx, id)
		if err != nil {
			return Observation{}, err
		}
		resolvable[id] = resolveAnchors(anchors, wanted[id])
	}
	relations, err := rel.ProvisionRelations(ctx)
	if err != nil {
		return Observation{}, err
	}
	var findings []Fired
	unresolved := map[string]bool{}
	for _, c := range scan.claims {
		var unmatched []string
		open := false
		for _, t := range c.targets {
			if !t.ok || !resolvable[t.document][t.anchor] {
				open = true
				continue
			}
			if !closedBy(relations[relationEnds{TargetDocument: t.document, TargetNode: t.anchor, Kind: c.kind}], c.subtree) {
				unmatched = append(unmatched, t.number+"|"+t.anchor)
			}
		}
		if len(unmatched) > 0 {
			slices.Sort(unmatched)
			findings = append(findings, Fired{Subject: c.subject(), Severity: c.severity, FingerprintInputs: []string{c.class, c.kind, c.hash, strings.Join(unmatched, ",")}})
		} else if open {
			unresolved[strings.Join(c.subject().Fields, "\x00")] = true
		}
	}
	return claimObservation(cov, scan, unresolved, findings), nil
}

func closedBy(sources, subtree []int64) bool {
	for _, source := range sources {
		if slices.Contains(subtree, source) {
			return true
		}
	}
	return false
}

func claimObservation(cov *coverage, scan claimScan, unresolved map[string]bool, findings []Fired) Observation {
	partial := map[string]bool{}
	for key := range scan.blocked {
		partial[key] = true
	}
	for key := range scan.unread {
		document, _, _ := strings.Cut(key, "\x00")
		partial[document] = true
	}
	for key := range unresolved {
		document, _, _ := strings.Cut(key, "\x00")
		partial[document] = true
	}
	obs := cov.done(findings)
	for document := range partial {
		if cov.documents[document] {
			obs.Observed--
			obs.NotObservable++
		}
	}
	obs.ObservedCompletely = func(s Subject) bool {
		if len(s.Fields) != 3 || !cov.documents[s.Fields[0]] || scan.blocked[s.Fields[0]] {
			return false
		}
		return !scan.unread[articleKey(s.Fields[0], s.Fields[1])] && !unresolved[strings.Join(s.Fields, "\x00")]
	}
	return obs
}

func resolveAnchors(anchors []releaseAnchor, requests []string) map[string]bool {
	ids := make(map[string]bool, len(anchors))
	siblings := map[string]bool{}
	for _, a := range anchors {
		ids[a.Anchor] = true
		if i := strings.LastIndexByte(a.Anchor, '~'); i >= 0 && digits(a.Anchor[i+1:]) {
			siblings[a.Anchor[:i]] = true
		}
	}
	out := map[string]bool{}
	for _, request := range requests {
		out[request] = buildResolves(ids, siblings, request)
	}
	return out
}

func buildResolves(ids, siblings map[string]bool, request string) bool {
	path := ""
	for i, component := range strings.Split(request, "/") {
		if i > 0 {
			path += "/"
		}
		path += component
		if siblings[path] {
			return false
		}
		if !ids[path] {
			return i > 0
		}
	}
	return true
}

func digits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
