package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixture(t *testing.T) (string, Manifest) {
	t.Helper()
	dir := t.TempDir()
	for _, p := range []string{"bundle.json", "tokenizer.json", "onnx/model_int8.onnx"} {
		b, err := os.ReadFile(filepath.Join("testdata", p))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, m
}

func TestLoad_RejectsUnknownFieldsAndBadRevision(t *testing.T) {
	data, err := os.ReadFile("testdata/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, data string }{
		{"unknown", strings.Replace(string(data), `"model":`, `"unknown": 1, "model":`, 1)},
		{"short", strings.Replace(string(data), "18b44161e041bf1d3a333ab5144b5b7b93f914d2", "18b44161e041", 1)},
		{"prefix", strings.Replace(string(data), "18b44161e041bf1d3a333ab5144b5b7b93f914d2", strings.Repeat("a", 40), 1)},
		{"model", strings.Replace(string(data), "AITeamVN/Vietnamese_Embedding_v2", "wrong", 1)},
		{"revision", strings.ReplaceAll(string(data), "18b44161e041", "aaaaaaaaaaaa")},
		{"trailing", string(data) + ` {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "bundle.json"), []byte(tc.data), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
}

func TestVerify_SizeHashAndMissingFile(t *testing.T) {
	for _, change := range []string{"size", "hash", "missing", "unlisted"} {
		t.Run(change, func(t *testing.T) {
			dir, m := fixture(t)
			switch change {
			case "size":
				m.Files[0].Size++
			case "hash":
				m.Files[0].SHA256 = strings.Repeat("0", 64)
			case "missing":
				if err := os.Remove(filepath.Join(dir, m.Files[0].Path)); err != nil {
					t.Fatal(err)
				}
			case "unlisted":
				if err := m.Verify(dir, []string{"absent"}); err == nil {
					t.Fatal("accepted unlisted")
				}
				return
			}
			if err := m.Verify(dir, []string{m.Files[0].Path}); err == nil {
				t.Fatal("accepted mismatch")
			}
		})
	}
	dir, m := fixture(t)
	if err := m.Verify(dir, []string{"tokenizer.json", QueryModelPath}); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_RejectsTraversalAbsoluteDuplicateAndSymlink(t *testing.T) {
	for _, p := range []string{"../outside", "/absolute", "onnx/../tokenizer.json", "onnx//model_int8.onnx"} {
		t.Run(p, func(t *testing.T) {
			dir, m := fixture(t)
			m.Files[0].Path = p
			if err := m.Verify(dir, []string{p}); err == nil {
				t.Fatal("accepted path")
			}
		})
	}
	dir, m := fixture(t)
	m.Files = append(m.Files, m.Files[0])
	if err := m.Verify(dir, []string{m.Files[0].Path}); err == nil {
		t.Fatal("accepted duplicate")
	}
	dir, m = fixture(t)
	if err := os.Remove(filepath.Join(dir, "onnx/model_int8.onnx")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../tokenizer.json", filepath.Join(dir, "onnx/model_int8.onnx")); err != nil {
		t.Fatal(err)
	}
	if err := m.Verify(dir, []string{QueryModelPath}); err == nil {
		t.Fatal("accepted symlink")
	}
	dir, m = fixture(t)
	if err := os.RemoveAll(filepath.Join(dir, "onnx")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", filepath.Join(dir, "onnx")); err != nil {
		t.Fatal(err)
	}
	if err := m.Verify(dir, []string{QueryModelPath}); err == nil {
		t.Fatal("accepted symlink directory")
	}
}

func TestVerify_IgnoresListedFilesNotLoaded(t *testing.T) {
	dir, m := fixture(t)
	m.Files = append(m.Files, File{Path: "model.safetensors", Size: 10, SHA256: strings.Repeat("0", 64)})
	if err := m.Verify(dir, []string{"tokenizer.json", QueryModelPath}); err != nil {
		t.Fatal(err)
	}
}

func TestQueryRecipe_HashesMatchPython(t *testing.T) {
	// python3 -c 'import json,hashlib; m=json.load(open("apps/queryembed/internal/bundle/testdata/bundle.json")); h=lambda x:hashlib.sha256(json.dumps(x,ensure_ascii=False,separators=(",",":"),sort_keys=True).encode()).hexdigest(); t=h([x for x in m["files"] if "token" in x["path"] or x["path"].endswith("sentencepiece.bpe.model")]); w=h([x for x in m["files"] if x["path"]=="onnx/model_int8.onnx"]); r=dict(model=m["model"],revision=m["revision"],resolved_revision=m["resolved_revision"],tokenizer_revision=m["resolved_revision"],tokenizer_sha256=t,weights_sha256=w,dimensions=1024,pooling="cls",normalization="l2-float32",input="raw",runtime="query",backend="onnxruntime-cpu",precision="int8",token_cap=256,special_tokens=True,truncation=False,language="go 1.27.3",tokenizer_library="daulet/tokenizers 1.27.0",runtime_library="onnxruntime 1.31.1"); print(t,w,hashlib.sha256(json.dumps(r,ensure_ascii=False,separators=(",",":")).encode()).hexdigest())'
	_, m := fixture(t)
	r, err := QueryRecipe(m, Versions{"1.27.3", "1.27.0", "1.31.1"})
	if err != nil {
		t.Fatal(err)
	}
	if r.TokenizerSHA256 != "5014994a30e823c64ad989c746ec475e97529ffe49beead072f3ac1639d89508" || r.WeightsSHA256 != "045fd479d7088ccd11cdfe8a316669ef30b6fb4805ac1c52330bf6138a4fe9a7" || r.Hash() != "e31c4809076acae41caa690a944835f7a61f39cb96f02785776d7294cd8ce603" {
		t.Fatalf("unexpected hashes: %+v %s", r, r.Hash())
	}
}
func TestQueryRecipe_FieldsAndVersions(t *testing.T) {
	_, m := fixture(t)
	r, err := QueryRecipe(m, Versions{"1.27.3", "1.27.0", "1.31.1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`"dimensions":1024`, `"pooling":"cls"`, `"normalization":"l2-float32"`, `"input":"raw"`, `"runtime":"query"`, `"backend":"onnxruntime-cpu"`, `"precision":"int8"`, `"token_cap":256`, `"special_tokens":true`, `"truncation":false`, `"language":"go 1.27.3"`, `"tokenizer_library":"daulet/tokenizers 1.27.0"`, `"runtime_library":"onnxruntime 1.31.1"`} {
		if !strings.Contains(string(b), s) {
			t.Errorf("missing %s", s)
		}
	}
	if r.Model != m.Model || r.Revision != m.Revision || r.ResolvedRevision != m.ResolvedRevision || r.TokenizerRevision != m.ResolvedRevision {
		t.Fatal("incorrect model identity")
	}
}
func TestQueryRecipe_RefusesWrongModelOrMissingFiles(t *testing.T) {
	_, m := fixture(t)
	for _, change := range []string{"model", "revision", "tokenizer", "weights"} {
		t.Run(change, func(t *testing.T) {
			n := m
			switch change {
			case "model":
				n.Model = "wrong"
			case "revision":
				n.Revision = "wrong"
			case "tokenizer":
				n.Files = []File{m.Files[0]}
			case "weights":
				n.Files = []File{m.Files[1]}
			}
			if _, err := QueryRecipe(n, Versions{}); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
}

func TestLoad_RejectsDuplicateAndCaseAliasKeys(t *testing.T) {
	data, err := os.ReadFile("testdata/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, old, replacement string }{
		{"root duplicate", `"model":`, `"model":"wrong", "model":`},
		{"root alias", `"model":`, `"Model":"wrong", "model":`},
		{"entry duplicate", `"path":`, `"path":"wrong", "path":`},
		{"entry alias", `"path":`, `"Path":"wrong", "path":`},
		{"entry unknown", `"path":`, `"extra":1, "path":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "bundle.json"), []byte(strings.Replace(string(data), tc.old, tc.replacement, 1)), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("accepted invalid keys")
			}
		})
	}
}

func TestVerify_RejectsFIFOWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(p, 0600); err != nil {
		t.Skipf("FIFO unsupported: %v", err)
	}
	m := Manifest{Files: []File{{Path: "fifo"}}}
	done := make(chan error, 1)
	go func() { done <- m.Verify(dir, []string{"fifo"}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("accepted FIFO")
		}
	case <-time.After(30 * time.Millisecond):
		fd, err := syscall.Open(p, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = syscall.Close(fd)
		}
		t.Fatal("verification blocked opening FIFO")
	}
}

func TestManifestHash_PythonGolden(t *testing.T) {
	// python3 -c 'import json,hashlib; items=[dict(path="tiếng Việt/<>&\u2028\u2029",sha256="a",size=9007199254740993),dict(path=r"literal\u2028",sha256="b",size=2),dict(path="third",sha256="c",size=3)]; print(hashlib.sha256(json.dumps(items,ensure_ascii=False,separators=(",",":"),sort_keys=True).encode()).hexdigest())'
	files := []File{{Path: "tiếng Việt/<>&\u2028\u2029", SHA256: "a", Size: 9007199254740993}, {Path: `literal\u2028`, SHA256: "b", Size: 2}, {Path: "third", SHA256: "c", Size: 3}}
	got, err := manifestHash(files)
	if err != nil {
		t.Fatal(err)
	}
	if got != "f2834e3123671813e10ab47957b5b7e33f98a67b58cd80761359d2b04a5fb2c2" {
		t.Fatalf("hash: %s", got)
	}
}
