package intake

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type rawIdentity struct {
	Key    string `yaml:"key"`
	Anchor string `yaml:"anchor"`
}

type rawRecord struct {
	Origin   string `yaml:"origin"`
	OriginID string `yaml:"origin_id"`
	Kind     string `yaml:"kind"`
	Target   struct {
		Key    string `yaml:"key"`
		Anchor string `yaml:"anchor"`
		Number string `yaml:"number"`
	} `yaml:"target"`
	Claim struct {
		Code   string      `yaml:"code"`
		Acting rawIdentity `yaml:"acting"`
	} `yaml:"claim"`
}

func ParseFile(country, path string) (File, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, nil, fmt.Errorf("read reports %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return File{}, nil, fmt.Errorf("read reports %q: %w", path, err)
	}
	file, err := Parse(country, data)
	if err != nil {
		return File{}, nil, err
	}
	return file, data, nil
}

func invalid(problems ...string) error { return &ValidationError{Problems: problems} }

func Parse(country string, data []byte) (File, error) {
	if !countryPattern.MatchString(country) {
		return File{}, fmt.Errorf("invalid country %q", country)
	}
	if len(data) > MaxFileBytes {
		return File{}, invalid("file exceeds 8 MiB")
	}
	if len(data) == 0 {
		return File{}, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var raw []rawRecord
	if err := decoder.Decode(&raw); err != nil {
		return File{}, invalid("decode: " + err.Error())
	}
	var further yaml.Node
	if err := decoder.Decode(&further); !errors.Is(err, io.EOF) {
		return File{}, invalid("further documents are refused")
	}
	if len(raw) > MaxRecords {
		return File{}, invalid(fmt.Sprintf("file has more than %d records", MaxRecords))
	}
	file := File{Records: make([]Record, 0, len(raw))}
	var problems []string
	seen := map[[2]string]bool{}
	for position, r := range raw {
		rec := Record{Origin: r.Origin, OriginID: r.OriginID, Kind: r.Kind, Target: Target(r.Target), Claim: Claim{Code: r.Claim.Code, Acting: Identity(r.Claim.Acting)}}
		for _, problem := range rec.validate(country) {
			problems = append(problems, fmt.Sprintf("record %d: %s", position, problem))
		}
		switch {
		case seen[rec.pair()]:
			problems = append(problems, fmt.Sprintf("record %d: duplicate origin and origin_id", position))
		case len(file.Records) > 0 && compare(file.Records[len(file.Records)-1], rec) > 0:
			problems = append(problems, fmt.Sprintf("record %d: records must be in origin and origin_id order", position))
		}
		seen[rec.pair()] = true
		file.Records = append(file.Records, rec)
	}
	if len(problems) > 0 {
		return File{}, &ValidationError{Problems: problems}
	}
	if !bytes.Equal(Encode(file.Records), data) {
		return File{}, invalid("file is not in canonical form: it must be exactly what corpus findings import --write produces, with no comments, no other spelling, and no anchors, aliases or tags")
	}
	return file, nil
}
