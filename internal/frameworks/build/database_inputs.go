package build

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"

	"danny.vn/trachuan/internal/frameworks/registry"
	"danny.vn/trachuan/internal/frameworks/relate"
)

type stepProvenance struct {
	Recipe    recipe
	InputHash string
}

type databaseInputs struct {
	versions []registry.Framework
	controls []Control
	chunks   []Chunk
	edges    []relate.Edge
	gaps     []Gap
	steps    map[string]stepProvenance
}

func readDatabaseInputs(base string) (databaseInputs, error) {
	var d databaseInputs
	reg, err := readRegistry(filepath.Join(base, "snapshot/sources/frameworks.yaml"))
	if err != nil {
		return d, err
	}
	policy, err := readFile(filepath.Join(base, "snapshot/gap-kinds.json"))
	if err != nil {
		return d, err
	}
	expected, err := json.Marshal(gapKinds())
	if err != nil {
		return d, hide("encode gap kind table", err)
	}
	if !equalJSON(policy, expected) {
		return d, fmt.Errorf("gap kind table changed")
	}
	d.versions = releaseVersions(reg)
	if d.controls, err = loadRows[Control](filepath.Join(base, "parse/controls.jsonl")); err != nil {
		return d, err
	}
	if d.chunks, err = loadRows[Chunk](filepath.Join(base, "chunk/chunks.jsonl")); err != nil {
		return d, err
	}
	if d.edges, err = loadRows[relate.Edge](filepath.Join(base, "relate/edges.jsonl")); err != nil {
		return d, err
	}
	d.steps = map[string]stepProvenance{}
	access := map[string]string{}
	for _, v := range d.versions {
		access[v.Code+":"+v.Version] = v.Access
	}
	seen := map[string]bool{}
	for _, step := range []string{"snapshot", "parse", "relate", "chunk", "embed"} {
		raw, err := readFile(filepath.Join(base, step, "manifest.json"))
		if err != nil {
			return d, err
		}
		var m manifest
		if err = json.Unmarshal(raw, &m); err != nil {
			return d, hide("decode step provenance", err)
		}
		d.steps[step] = stepProvenance{m.Recipe, stepInputHash(m.Inputs)}
		gaps, err := loadRows[Gap](filepath.Join(base, step, "gaps.jsonl"))
		if err != nil {
			return d, err
		}
		for _, g := range gaps {
			if _, _, err = classifyGap(g.Kind, access[g.Code+":"+g.Version]); err != nil {
				return d, err
			}
			if g.Count <= 0 {
				return d, fmt.Errorf("invalid gap count")
			}
			key, _ := json.Marshal([]string{g.Kind, g.Code, g.Version, gapSubject(g), strconv.Itoa(g.Count)})
			if seen[string(key)] {
				return d, fmt.Errorf("duplicate gap key")
			}
			seen[string(key)] = true
			d.gaps = append(d.gaps, g)
		}
	}
	return d, nil
}

func gapSubject(g Gap) string {
	if g.Citation != nil {
		return *g.Citation
	}
	return "fw:" + g.Code + ":" + g.Version
}
