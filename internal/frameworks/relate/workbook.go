package relate

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"danny.vn/trachuan/internal/frameworks/parse"
	"github.com/xuri/excelize/v2"
)

var (
	csfCitationPattern  = regexp.MustCompile(`^[A-Z]{2}\.[A-Z]{2,}-[0-9]+$`)
	isoCitationPattern  = regexp.MustCompile(`^(A\.)?[0-9]+(\.[0-9]+)*$`)
	nistCitationPattern = regexp.MustCompile(`^([A-Z]{2})-0*([0-9]+)(?:\(0*([0-9]+)\))?$`)
)

func Workbook(parser string, data []byte, nodes []parse.Node) ([]Input, int, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 256 << 20, UnzipXMLSizeLimit: 256 << 20})
	if err != nil {
		return nil, 0, fmt.Errorf("invalid mapping workbook")
	}
	defer func() { _ = f.Close() }()
	var inputs []Input
	dropped := 0
	found := false
	for _, sheet := range f.GetSheetList() {
		if parser == "nist-olir" {
			if sheet == "Definitions" {
				continue
			}
		} else if !strings.HasPrefix(sheet, "All CIS") {
			continue
		}
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid mapping sheet")
		}
		if len(rows) == 0 {
			continue
		}
		if parser == "nist-olir" {
			headers := map[string]int{}
			for i, h := range rows[0] {
				headers[strings.ToLower(strings.Join(strings.Fields(h), " "))] = i
			}
			focal, ok1 := headers["focal document element"]
			target, ok2 := headers["reference document element"]
			notes, ok3 := headers["comments (optional)"]
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			found = true
			for _, row := range rows[1:] {
				from := nistCitation(cellValue(row, focal))
				if from == "" {
					continue
				}
				to := cellValue(row, target)
				if !isoCitation(to) {
					dropped++
					continue
				}
				inputs = append(inputs, Input{From: Key{"nist80053", "r5", from}, To: Key{"iso27001", "2022", to}, Relation: "related", Source: "nist-olir", Evidence: cellValue(row, notes)})
			}
			continue
		}
		found = true
		var code, version string
		switch parser {
		case "cis-iso27001":
			code, version = "iso27001", "2022"
		case "cis-nistcsf":
			code, version = "nistcsf", "2.0"
		case "cis-nist80053":
			code, version = "nist80053", "r5"
		default:
			return nil, 0, fmt.Errorf("unknown mapping parser")
		}
		titles := map[string]string{}
		ambiguous := map[string]bool{}
		for _, n := range nodes {
			if n.Kind != "safeguard" {
				continue
			}
			key := titleKey(n.PublisherTitle)
			if prior, ok := titles[key]; ok && prior != n.Citation {
				ambiguous[key] = true
			}
			titles[key] = n.Citation
		}
		for _, row := range rows[1:] {
			relation, target, title := strings.ToLower(cellValue(row, 10)), cellValue(row, 11), titleKey(cellValue(row, 5))
			if relation == "" && target == "" {
				continue
			}
			switch relation {
			case "":
				relation = "related"
			case "equivalent", "subset", "superset":
			default:
				dropped++
				continue
			}
			citation := target
			switch code {
			case "iso27001":
				if strings.HasPrefix(citation, "A") && !strings.HasPrefix(citation, "A.") {
					citation = "A." + citation[1:]
				}
				if !isoCitation(citation) {
					citation = ""
				}
			case "nist80053":
				citation = nistCitation(citation)
			case "nistcsf":
				if !csfCitationPattern.MatchString(citation) {
					citation = ""
				}
			}
			if citation == "" || titles[title] == "" || ambiguous[title] {
				dropped++
				continue
			}
			inputs = append(inputs, Input{From: Key{"ciscontrols", "v8.1", titles[title]}, To: Key{code, version, citation}, Relation: relation, Source: "cis-v8.1-mappings"})
		}
	}
	if !found {
		return nil, 0, fmt.Errorf("missing mapping sheet")
	}
	return inputs, dropped, nil
}
func cellValue(row []string, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}
func titleKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func isoCitation(s string) bool {
	return isoCitationPattern.MatchString(s)
}
func nistCitation(s string) string {
	m := nistCitationPattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil {
		return ""
	}
	out := m[1] + "-" + m[2]
	if m[3] != "" {
		out += "(" + m[3] + ")"
	}
	return out
}
