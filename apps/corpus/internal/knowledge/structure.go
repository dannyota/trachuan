package knowledge

type effectPair struct{ actingKey, actingAnchor, targetKey, targetAnchor string }

type relationIdentity struct{ actingKey, targetKey, kind string }

type decisionIdentity struct {
	finding, fingerprint, firstRel string
	semantics                      int
}

type batchUse struct{ read, of, count int }

type structure struct {
	p         *parser
	lastID    string
	seen      map[string]bool
	effects   map[effectPair]string
	relations map[relationIdentity]bool
	decisions map[decisionIdentity]bool
	batches   map[string]*batchUse
}

func newStructure(p *parser) *structure {
	return &structure{p: p, seen: map[string]bool{}, effects: map[effectPair]string{}, relations: map[relationIdentity]bool{}, decisions: map[decisionIdentity]bool{}, batches: map[string]*batchUse{}}
}

func (s *structure) id(at, id string) {
	switch {
	case !idPattern.MatchString(id):
		s.p.add("%s: invalid id", at)
		return
	case s.seen[id]:
		s.p.add("%s: duplicate id", at)
	case id < s.lastID:
		s.p.add("%s: entries must be in id order", at)
	}
	s.seen[id] = true
	if id > s.lastID {
		s.lastID = id
	}
}

func (s *structure) decision(at string, d ReviewDecision) {
	identity := decisionIdentity{d.Finding, d.Fingerprint, d.FirstRel, d.Semantics}
	if s.decisions[identity] {
		s.p.add("%s: same decision as an earlier entry", at)
	}
	s.decisions[identity] = true
	s.batch(at, d.Reviewed)
}

func (s *structure) claim(at string, c Claim) {
	s.batch(at, c.Reviewed)
	if c.Kind == KindRelation {
		identity := relationIdentity{c.ActingKey, c.TargetKey, c.RelationKind}
		if s.relations[identity] {
			s.p.add("%s: same identity as an earlier entry", at)
		}
		s.relations[identity] = true
		return
	}
	pair := effectPair{c.ActingKey, c.ActingAnchor, c.TargetKey, c.TargetAnchor}
	switch previous, found := s.effects[pair]; {
	case !found:
		s.effects[pair] = c.RelationKind
	case previous == c.RelationKind:
		s.p.add("%s: same identity as an earlier entry", at)
	default:
		s.p.add("%s: same provisions as an earlier entry with different kinds", at)
	}
}

func (s *structure) batch(at string, r Reviewed) {
	if r.Batch == "" {
		return
	}
	use := s.batches[r.Batch]
	if use == nil {
		use = &batchUse{read: r.SampleRead, of: r.SampleOf}
		s.batches[r.Batch] = use
	}
	use.count++
	if use.read != r.SampleRead || use.of != r.SampleOf {
		s.p.add("%s: batch %q disagrees on its sample numbers", at, r.Batch)
	}
	if use.count > use.of {
		s.p.add("%s: batch %q has more entries than sample_of", at, r.Batch)
	}
}
