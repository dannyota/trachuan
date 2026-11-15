package command

import (
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/registry"
	"danny.vn/trachuan/internal/frameworks/relate"
)

func Run(args []string, out io.Writer) error {
	if len(args) != 2 && (len(args) != 1 || args[0] != "edges") {
		return fmt.Errorf("usage: frameworks <code> <version> | frameworks edges")
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

	if len(args) == 1 {
		return edgeStats(reg, out)
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

func edgeStats(reg *registry.Registry, out io.Writer) error {
	catalogs := map[string]*parse.Result{}
	for _, v := range reg.Frameworks {
		for _, file := range v.Files {
			if file.Role != "catalog" || v.Parser == "" {
				continue
			}
			data, err := reg.Read(file.Path)
			if err != nil {
				return fmt.Errorf("read catalog: %w", err)
			}
			result, err := parse.Parse(v.Parser, data, v.Code, v.Version, v.ExpectedControls)
			if err != nil {
				return fmt.Errorf("parse catalog: %w", err)
			}
			catalogs[v.Code+":"+v.Version] = result
		}
	}
	var inputs []relate.Input
	dropped := 0
	for _, v := range reg.Frameworks {
		for _, file := range v.Files {
			if file.Role != "mapping workbook" {
				continue
			}
			data, err := reg.Read(file.Path)
			if err != nil {
				return fmt.Errorf("read mapping workbook: %w", err)
			}
			var nodes []parse.Node
			if result := catalogs[v.Code+":"+v.Version]; result != nil {
				nodes = result.Nodes
			}
			rows, count, err := relate.Workbook(file.Parser, data, nodes)
			if err != nil {
				return fmt.Errorf("parse mapping workbook: %w", err)
			}
			inputs = append(inputs, rows...)
			dropped += count
		}
	}
	edges, stats, err := relate.Build(reg, catalogs, inputs)
	if err != nil {
		return fmt.Errorf("build edges: %w", err)
	}
	stats.Dropped += dropped
	return relate.Report(out, edges, stats, reg)
}
