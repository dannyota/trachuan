package build

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type goldenReviewEntry struct {
	Release    *string   `yaml:"release"`
	Reviewer   *string   `yaml:"reviewer"`
	Date       *string   `yaml:"date"`
	CSVHash    *string   `yaml:"csv_sha256"`
	Added      *[]string `yaml:"added"`
	Rejected   *[]string `yaml:"rejected"`
	Retargeted *[]string `yaml:"retargeted"`
	Note       *string   `yaml:"note"`
}

func validateGoldenReview(dir string) error {
	csv, err := readFile(filepath.Join(dir, "trachuan/golden.csv"))
	if errors.Is(err, os.ErrNotExist) {
		csv = nil
	} else if err != nil {
		return err
	}
	raw, err := readFile(filepath.Join(dir, "trachuan/golden-review.yaml"))
	if errors.Is(err, os.ErrNotExist) && csv == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("golden CSV has no matching review")
	}
	if err != nil {
		return err
	}
	var document yaml.Node
	if yaml.Unmarshal(raw, &document) != nil || !plainReviewYAML(&document) {
		return fmt.Errorf("invalid golden review YAML")
	}
	var file struct {
		Reviews *[]goldenReviewEntry `yaml:"reviews"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if dec.Decode(&file) != nil || dec.Decode(new(any)) != io.EOF || file.Reviews == nil {
		return fmt.Errorf("invalid golden review YAML")
	}
	matched := false
	for _, entry := range *file.Reviews {
		if !validGoldenReview(entry) {
			return fmt.Errorf("invalid golden review entry")
		}
		matched = matched || *entry.CSVHash == digest(csv)
	}
	if csv != nil && !matched {
		return fmt.Errorf("golden CSV has no matching review")
	}
	return nil
}

func plainReviewYAML(node *yaml.Node) bool {
	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Tag == "!!merge" {
		return false
	}
	for _, child := range node.Content {
		if !plainReviewYAML(child) {
			return false
		}
	}
	return true
}

func validGoldenReview(e goldenReviewEntry) bool {
	if e.Release == nil || e.Reviewer == nil || e.Date == nil || e.CSVHash == nil || e.Added == nil || e.Rejected == nil || e.Retargeted == nil || e.Note == nil {
		return false
	}
	if !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(*e.Release) || *e.Reviewer != "coordinator" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(*e.CSVHash) || strings.ContainsAny(*e.Note, "\r\n") {
		return false
	}
	date, err := time.Parse("2006-01-02", *e.Date)
	if err != nil || date.Format("2006-01-02") != *e.Date {
		return false
	}
	for _, ids := range [][]string{*e.Added, *e.Rejected, *e.Retargeted} {
		for _, id := range ids {
			if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\r\n") {
				return false
			}
		}
	}
	return true
}
