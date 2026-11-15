package command

import (
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/registry"
)

func Run(args []string, out io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: frameworks <code> <version>")
	}
	f, err := os.Open("sources/frameworks.yaml")
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer func() { _ = f.Close() }()
	reg, err := registry.Parse(f)
	if err != nil {
		return fmt.Errorf("read registry: %w", err)
	}

	targets := map[string]string{}
	for _, framework := range reg.Frameworks {
		targets[framework.Code] = framework.Code
		for _, alias := range framework.Aliases {
			targets[alias] = framework.Code
		}
	}
	for _, v := range reg.Frameworks {
		if v.Code != args[0] || v.Version != args[1] {
			continue
		}
		for _, file := range v.Files {
			if file.Role != "catalog" {
				continue
			}
			data, err := reg.Read(file.Path)
			if err != nil {
				return fmt.Errorf("read catalog: %w", err)
			}
			result, err := parse.Parse(v.Parser, data, v.Code, v.Version, v.ExpectedControls)
			if result != nil {
				if reportErr := parse.Report(out, result, targets); reportErr != nil {
					return fmt.Errorf("write statistics: %w", reportErr)
				}
			}
			if err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("catalog not registered")
	}
	return fmt.Errorf("version not registered")
}
