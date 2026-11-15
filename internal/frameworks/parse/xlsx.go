package parse

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

func rows(data []byte, sheet string, contains bool) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 256 << 20, UnzipXMLSizeLimit: 256 << 20})
	if err != nil {
		return nil, fmt.Errorf("invalid workbook")
	}
	defer func() { _ = f.Close() }()
	for _, name := range f.GetSheetList() {
		if name == sheet || (contains && strings.Contains(name, sheet)) {
			r, err := f.GetRows(name)
			if err != nil {
				return nil, fmt.Errorf("invalid workbook sheet")
			}
			return r, nil
		}
	}
	return nil, fmt.Errorf("missing workbook sheet")
}
func cell(row []string, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}
