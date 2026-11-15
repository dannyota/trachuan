package registry

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"gopkg.in/yaml.v3"
)

type Registry struct {
	Frameworks []Framework `yaml:"frameworks"`
}
type Framework struct {
	Code             string   `yaml:"code"`
	Version          string   `yaml:"version"`
	Name             string   `yaml:"name"`
	Publisher        string   `yaml:"publisher"`
	Language         string   `yaml:"language"`
	Access           string   `yaml:"access"`
	LicenceKind      string   `yaml:"licence_kind"` //nolint:misspell // Registry field follows the design.
	LicenceURL       string   `yaml:"licence_url"`  //nolint:misspell // Registry field follows the design.
	Released         *string  `yaml:"released"`
	Ordinal          int      `yaml:"ordinal"`
	CitationScheme   string   `yaml:"citation_scheme"`
	Aliases          []string `yaml:"aliases"`
	Files            []File   `yaml:"files"`
	Parser           string   `yaml:"parser"`
	ExpectedControls *int     `yaml:"expected_controls"`
}
type File struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
	Size   int64  `yaml:"size"`
	Role   string `yaml:"role"`
	Format string `yaml:"format"`
}

func Parse(r io.Reader) (*Registry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("decode registry: %w", err)
	}
	if hasMerge(&node) {
		return nil, fmt.Errorf("registry merge keys are not supported")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var reg Registry
	if err := dec.Decode(&reg); err != nil {
		return nil, fmt.Errorf("decode registry: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple registry documents")
		}
		return nil, fmt.Errorf("decode registry: %w", err)
	}
	if err := reg.Validate(); err != nil {
		return nil, fmt.Errorf("validate registry: %w", err)
	}
	return &reg, nil
}

func normalized(s string) string { return cases.Fold().String(strings.Join(strings.Fields(s), " ")) }

func (r *Registry) Validate() error {
	codePattern := regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	versionPattern := regexp.MustCompile(`^[0-9a-z]+([.+-][0-9a-z]+)*$`)
	hashPattern := regexp.MustCompile(`^[0-9a-f]{64}$`)
	versions := map[string]bool{}
	paths := map[string]bool{}
	hashes := map[string]bool{}
	orders := map[string]bool{}
	aliases := map[string]string{}
	codes := map[string]bool{}
	for _, f := range r.Frameworks {
		codes[normalized(f.Code)] = true
	}
	for _, f := range r.Frameworks {
		if !codePattern.MatchString(f.Code) || !versionPattern.MatchString(f.Version) {
			return fmt.Errorf("invalid framework identity %q %q", f.Code, f.Version)
		}
		key := f.Code + "\x00" + f.Version
		if versions[key] {
			return fmt.Errorf("duplicate version %q", key)
		}
		versions[key] = true
		if f.Name == "" || f.Publisher == "" || f.Language == "" || f.CitationScheme == "" {
			return fmt.Errorf("missing framework metadata for %s", f.Code)
		}
		if f.Access != "free" && f.Access != "licensed" {
			return fmt.Errorf("invalid access for %s", f.Code)
		}
		if f.Access == "free" {
			u, err := url.Parse(f.LicenceURL)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || !openLicence(f.LicenceKind) {
				return fmt.Errorf("invalid free license for %s", f.Code)
			}
		}
		if f.ExpectedControls != nil && *f.ExpectedControls <= 0 {
			return fmt.Errorf("invalid count for %s", f.Code)
		}
		date := ""
		if f.Released != nil {
			date = *f.Released
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return fmt.Errorf("invalid release date: %w", err)
			}
		}
		if f.Ordinal <= 0 {
			return fmt.Errorf("invalid ordinal for %s", f.Code)
		}
		order := fmt.Sprintf("%s\x00%s\x00%d", f.Code, date, f.Ordinal)
		if orders[order] {
			return fmt.Errorf("duplicate version order for %s", f.Code)
		}
		orders[order] = true
		local := map[string]bool{}
		for _, a := range f.Aliases {
			n := normalized(a)
			if n == "" || codes[n] || local[n] {
				return fmt.Errorf("invalid alias %q", a)
			}
			if owner, ok := aliases[n]; ok && owner != f.Code {
				return fmt.Errorf("duplicate alias %q", a)
			}
			aliases[n] = f.Code
			local[n] = true
		}
		for _, file := range f.Files {
			if !validPath(file.Path) || paths[file.Path] || hashes[file.SHA256] || !hashPattern.MatchString(file.SHA256) || file.Size <= 0 || file.Format == "" {
				return fmt.Errorf("invalid file %q", file.Path)
			}
			switch file.Role {
			case "text", "catalog", "mapping workbook", "guide", "change log":
			default:
				return fmt.Errorf("invalid role %q", file.Role)
			}
			paths[file.Path] = true
			hashes[file.SHA256] = true
		}
	}
	if len(r.Frameworks) == 0 {
		return fmt.Errorf("empty registry")
	}
	return nil
}

func openLicence(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "public-domain", "cc0", "cc0 1.0", "cc0-1.0", "cc by", "cc by 4.0", "cc by-sa", "cc by-sa 4.0", "cc-by", "cc-by-4.0", "cc-by-sa", "cc-by-sa-4.0":
		return true
	}
	return false
}

func hasMerge(node *yaml.Node) bool {
	if node.Tag == "!!merge" {
		return true
	}
	for _, child := range node.Content {
		if hasMerge(child) {
			return true
		}
	}
	return false
}
