package fetch

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestFetch_StoredDetailWithoutViewCount(t *testing.T) {
	cases := []struct{ name, detail, want string }{
		{"first number", `{"success":true,"data":{"viewCount":1,"id":"66518"}}`, `{"success":true,"data":{"id":"66518"}}`},
		{"middle null", `{"success":true,"data":{"id":"66518","viewCount":null,"other":1}}`, `{"success":true,"data":{"id":"66518","other":1}}`},
		{"last string", `{"success":true,"data":{"id":"66518","viewCount":"12"}}`, `{"success":true,"data":{"id":"66518"}}`},
		{"whitespace", "{\n \"success\": true,\n \"data\": {\n  \"viewCount\" : 10,\n  \"id\": \"66518\"\n }\n}", "{\n \"success\": true,\n \"data\": {\n  \n  \"id\": \"66518\"\n }\n}"},
		{"nested and string", `{"success":true,"viewCount":9,"data":{"id":"66518","nested":{"viewCount":3},"text":"viewCount: 4","viewCount":2}}`, `{"success":true,"viewCount":9,"data":{"id":"66518","nested":{"viewCount":3},"text":"viewCount: 4"}}`},
		{"absent", `{"success":true,"data":{"id":"66518"}}`, `{"success":true,"data":{"id":"66518"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, env := setup(t, &fixtureServer{detailBody: tc.detail})
			out := process(t, s, env)
			got, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "detail.json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("detail = %q, want %q", got, tc.want)
			}
			obs := out.Observations[0]
			if obs.ContentSHA256 != fmt.Sprintf("%x", sha256.Sum256(got)) || obs.Size != int64(len(got)) {
				t.Errorf("observation = %+v", obs)
			}
		})
	}
}

func TestFetch_ViewCountDoesNotChangeBundle(t *testing.T) {
	f := &fixtureServer{detailBody: `{"success":true,"data":{"id":"66518","viewCount":1}}`}
	s, env := setup(t, f)
	a := process(t, s, env)
	f.detailBody = `{"success":true,"data":{"id":"66518","viewCount":999}}`
	b := process(t, s, env)
	if a.Artifact.SHA256 != b.Artifact.SHA256 {
		t.Fatal("counter changed bundle hash")
	}
}

func TestFetch_RecipeChangesForViewCount(t *testing.T) {
	old := stage.RecipeHash("fetch", "vbpl-fetch/1", "chinhphu-fetch/2", "json:encoding/json:compact:no-newline", "metadata:version,source_id,source_url,occurrences,attachments;occurrence:name,value,cells,ambiguous;attachment:name,label,url", "document:version,document;fields:key,country,source,source_id,number,title,doc_type,issuer,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,is_consolidated,has_content,source_url,identity_evidence", "files:version,files;file:source,name,sha256,size,path,url,role;report:version,missing,too_large;outcome:name,url;evidence:path,sha256")
	if (&Stage{}).Recipe(stage.Snapshot{}) == old {
		t.Fatal("recipe hash did not change")
	}
}

func TestRemoveViewCount_UnchangedAndMalformed(t *testing.T) {
	for _, input := range []string{`{"data":null}`, `{"data":[{"viewCount":1}]}`, `{"data":{"nested":{"viewCount":1}}}`} {
		got, err := removeViewCount([]byte(input))
		if err != nil || string(got) != input {
			t.Errorf("input %q: got %q, %v", input, got, err)
		}
	}
	for _, input := range []string{`{"data":{"viewCount":1`, `{"data":{} } trailing`, `{"data":{"viewCount":1,}}`} {
		if _, err := removeViewCount([]byte(input)); err == nil {
			t.Errorf("accepted malformed input %q", input)
		}
	}
}

func TestRemoveViewCount_ExactDeletedSpan(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"first compact", `{"data":{"viewCount":1,"x":0}}`, `{"data":{"x":0}}`},
		{"middle compact", `{"data":{"x":0,"viewCount":1,"y":2}}`, `{"data":{"x":0,"y":2}}`},
		{"last compact", `{"data":{"x":0,"viewCount":1}}`, `{"data":{"x":0}}`},
		{"sole compact", `{"data":{"viewCount":1}}`, `{"data":{}}`},
		{"first whitespace", "{\"data\":{\n  \"viewCount\" : 1  ,\n  \"x\":0\n}}", "{\"data\":{\n  \n  \"x\":0\n}}"},
		{"middle whitespace", "{\"data\":{\n  \"x\":0 ,\n  \"viewCount\":1  ,\n  \"y\":2\n}}", "{\"data\":{\n  \"x\":0 ,\n  \n  \"y\":2\n}}"},
		{"last whitespace", "{\"data\":{\n  \"x\":0 \n ,\n  \"viewCount\":1 \n}}", "{\"data\":{\n  \"x\":0 \n  \n}}"},
		{"sole whitespace", "{\"data\":{\n  \"viewCount\":1 \n}}", "{\"data\":{\n   \n}}"},
		{"duplicate", `{"data":{"x":0,"viewCount":1 ,"viewCount":2}}`, `{"data":{"x":0}}`},
		{"duplicates last", `{"data":{"x":0,"viewCount":1,"viewCount":2}}`, `{"data":{"x":0}}`},
		{"duplicates sole", `{"data":{"viewCount":1,"viewCount":2}}`, `{"data":{}}`},
		{"duplicates last whitespace", "{\"data\":{\"x\":0 \n , \n \"viewCount\":1 , \n \"viewCount\":2 \n}}", "{\"data\":{\"x\":0 \n  \n}}"},
		{"duplicates sole whitespace", "{\"data\":{ \n \"viewCount\":1 , \n \"viewCount\":2 \n}}", "{\"data\":{ \n  \n  \n}}"},
		{"escaped key", `{"data":{"view\u0043ount":1,"x":0}}`, `{"data":{"x":0}}`},
		{"nested and string", `{"viewCount":1,"data":{"x":{"viewCount":2},"s":"viewCount","viewCount":3}}`, `{"viewCount":1,"data":{"x":{"viewCount":2},"s":"viewCount"}}`},
		{"multiple data", `{"data":{"viewCount":1,"a":1},"data":null,"data":{"b":2,"viewCount":3}}`, `{"data":{"a":1},"data":null,"data":{"b":2}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := removeViewCount([]byte(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCutViewCount_DecoderErrors(t *testing.T) {
	for _, input := range []string{``, `{1}`, `{"other":`, `{"other":1`} {
		if _, _, err := cutViewCount([]byte(input)); err == nil {
			t.Errorf("accepted malformed object %q", input)
		}
	}
}

func TestRemoveViewCount_ManyDuplicates(t *testing.T) {
	input := `{"data":{"x":0,` + strings.Repeat(`"viewCount":1,`, 100000) + `"y":2}}`
	got, err := removeViewCount([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"data":{"x":0,"y":2}}` {
		t.Fatalf("got %q", got)
	}
}
