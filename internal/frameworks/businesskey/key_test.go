package businesskey

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestBusinessKeyValidator_Forms(t *testing.T) {
	validator := New()
	for _, tc := range []struct {
		field, value string
		valid        bool
	}{
		{"code", "a", true}, {"code", "fw-alpha3", true}, {"code", "FW-alpha", false}, {"code", "1alpha", false}, {"code", "fw--alpha", false}, {"code", "fw-alpha-", false}, {"code", "fw_alpha", false}, {"code", "fw-alpha\n", false}, {"code", "", false},
		{"version", "1.0+alpha-2", true}, {"version", "draft", true}, {"version", "V1", false}, {"version", "1..0", false}, {"version", "1_0", false}, {"version", "1.0\n", false}, {"version", "", false},
		{"citation", "A", true}, {"citation", "AC-2(1)", true}, {"citation", "A 1", true}, {"citation", "!~", true}, {"citation", strings.Repeat("X", 64), true},
		{"citation", strings.Repeat("X", 65), false}, {"citation", " A.1", false}, {"citation", "A.1 ", false}, {"citation", "A  1", false}, {"citation", "A\t1", false}, {"citation", "A\n1", false}, {"citation", "Đ.1", false}, {"citation", "A\x7f", false}, {"citation", "", false},
	} {
		t.Run(tc.field+"/"+tc.value, func(t *testing.T) {
			code, version, citation := "fw-alpha", "v1", "A.1"
			switch tc.field {
			case "code":
				code = tc.value
			case "version":
				version = tc.value
			case "citation":
				citation = tc.value
			}
			require.Equal(t, tc.valid, validator.Valid(code, version, citation))
		})
	}
}

func TestBusinessKeyForms_MatchesEmbeddedSchema(t *testing.T) {
	capture := &schemaCapture{}
	require.NoError(t, schema.ApplyTables(context.Background(), capture, 1024))
	sql := capture.sql
	for _, tc := range []struct{ name, column, pattern string }{
		{"framework_code_check", "code", keyCodePattern},
		{"framework_version_version_check", "version", keyVersionPattern},
		{"control_mapping_from_code_check", "from_code", keyCodePattern},
		{"control_mapping_to_code_check", "to_code", keyCodePattern},
		{"control_mapping_from_version_check", "from_version", keyVersionPattern},
		{"control_mapping_to_version_check", "to_version", keyVersionPattern},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := regexp.MustCompile(`CONSTRAINT\s+` + tc.name + `\s+CHECK\s*\(\s*` + tc.column + `\s*~\s*'([^']+)'\s*\)`).FindStringSubmatch(sql)
			require.Len(t, parts, 2)
			require.Equal(t, tc.pattern, parts[1])
		})
	}
	for _, tc := range []struct{ name, column string }{
		{"control_citation_check", "citation"},
		{"control_mapping_from_citation_check", "from_citation"},
		{"control_mapping_to_citation_check", "to_citation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := regexp.MustCompile(`CONSTRAINT\s+` + tc.name + `\s+CHECK\s*\(\s*octet_length\(` + tc.column + `\)\s+BETWEEN\s+(\d+)\s+AND\s+(\d+)\s+AND\s+` + tc.column + `\s*~\s*'([^']+)'\s*\)`).FindStringSubmatch(sql)
			require.Len(t, parts, 4)
			minimum, err := strconv.Atoi(parts[1])
			require.NoError(t, err)
			maximum, err := strconv.Atoi(parts[2])
			require.NoError(t, err)
			require.Equal(t, keyCitationMinBytes, minimum)
			require.Equal(t, keyCitationMaxBytes, maximum)
			require.Equal(t, keyCitationPattern, parts[3])
		})
	}
}

type schemaCapture struct{ sql string }

func (c *schemaCapture) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	c.sql += sql
	return pgconn.CommandTag{}, nil
}
