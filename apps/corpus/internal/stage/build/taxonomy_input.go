package build

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type taxonomySeed struct{ ItemID, Number, Code string }

// TaxonomyInput holds the pinned taxonomy files and resolved seed identities.
type TaxonomyInput struct {
	mapping                                taxonomy.Mapping
	custom                                 []taxonomy.CustomNode
	overrides                              []taxonomy.Override
	seeds                                  []taxonomySeed
	version, dir, packagePath, packageHash string
	files                                  [3][]byte
}

// ReadTaxonomy reads and validates taxonomy inputs and resolves package articles.
func ReadTaxonomy(dir, packagePath string) (TaxonomyInput, error) {
	input := TaxonomyInput{dir: dir, packagePath: packagePath}
	mapping, err := os.ReadFile(filepath.Join(dir, "mapping.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return input, nil
	}
	if err != nil {
		return TaxonomyInput{}, fmt.Errorf("read taxonomy mapping: %w", err)
	}
	input.files[0] = mapping
	for i, name := range []string{"custom.yaml", "overrides.yaml"} {
		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(readErr, os.ErrNotExist) {
			data = []byte("[]\n")
		} else if readErr != nil {
			return TaxonomyInput{}, fmt.Errorf("read taxonomy %s: %w", name, readErr)
		}
		input.files[i+1] = data
	}
	if input.mapping, err = taxonomy.ReadMapping(mapping); err != nil {
		return TaxonomyInput{}, fmt.Errorf("read mapping: %w", err)
	}
	if input.custom, err = taxonomy.ReadCustom(input.files[1]); err != nil {
		return TaxonomyInput{}, err
	}
	if input.overrides, err = taxonomy.ReadOverrides(input.files[2]); err != nil {
		return TaxonomyInput{}, err
	}
	if err = taxonomy.Validate(input.mapping, input.custom, input.overrides); err != nil {
		return TaxonomyInput{}, fmt.Errorf("validate taxonomy: %w", err)
	}
	if packagePath == "" {
		return TaxonomyInput{}, fmt.Errorf("taxonomy package is required")
	}
	pkg, err := taxonomy.ReadPackage(packagePath)
	if err != nil {
		return TaxonomyInput{}, fmt.Errorf("read taxonomy package: %w", err)
	}
	if pkg.SHA256 != input.mapping.PackageSHA256 {
		return TaxonomyInput{}, fmt.Errorf("taxonomy package hash mismatch")
	}
	input.packageHash = pkg.SHA256
	input.version = taxonomy.Version(input.files[0], input.files[1], input.files[2])
	resolver := taxonomy.NewArticleResolver(pkg, input.mapping)
	for _, article := range pkg.Articles {
		if article.ItemID == "" || article.Number == "" {
			continue
		}
		digits := true
		for _, c := range article.Number {
			if c < '0' || c > '9' {
				digits = false
				break
			}
		}
		code, ok := resolver.Code(article)
		if digits && ok {
			input.seeds = append(input.seeds, taxonomySeed{article.ItemID, article.Number, code})
		}
	}
	return input, nil
}

func (input TaxonomyInput) pinned() error {
	if input.dir == "" {
		return nil
	}
	for i, name := range []string{"mapping.yaml", "custom.yaml", "overrides.yaml"} {
		data, err := os.ReadFile(filepath.Join(input.dir, name))
		if i == 0 && input.files[0] == nil && err == nil {
			return fmt.Errorf("taxonomy mapping bytes changed")
		}
		if errors.Is(err, os.ErrNotExist) {
			if i == 0 {
				data = nil
			} else {
				data = []byte("[]\n")
			}
		} else if err != nil {
			return fmt.Errorf("read taxonomy %s: %w", name, err)
		}
		if i > 0 && input.files[0] == nil {
			continue
		}
		if !bytes.Equal(data, input.files[i]) {
			return fmt.Errorf("taxonomy %s bytes changed", name)
		}
	}
	if input.version != "" {
		data, err := os.ReadFile(input.packagePath)
		if err != nil {
			return fmt.Errorf("read taxonomy package: %w", err)
		}
		if hashBytes(data) != input.packageHash {
			return fmt.Errorf("taxonomy package bytes changed")
		}
	}
	return nil
}
