package evidence

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	identityFile    = "identity.jsonl"
	changesFile     = "changes.jsonl"
	summarySchema   = 1
	lineLimit       = 1 << 20
	recordDocument  = "document"
	recordRelation  = "relation"
	recordProvision = "provision"
)

type identityBindings struct {
	Rel                 string `json:"rel"`
	DumpSHA256          string `json:"dump_sha256"`
	BuildArtifactSHA256 string `json:"build_artifact_sha256"`
	ReportSHA256        string `json:"report_sha256"`
}

type changesBindings struct {
	identityBindings
	PredecessorRel            string `json:"predecessor_rel"`
	PredecessorIdentitySHA256 string `json:"predecessor_identity_sha256"`
}

type identityHeader struct {
	Schema   int              `json:"schema"`
	Bindings identityBindings `json:"bindings"`
}

type changesHeader struct {
	Schema   int             `json:"schema"`
	Bindings changesBindings `json:"bindings"`
	Baseline bool            `json:"baseline"`
}

type validityRow struct {
	StatusClass string  `json:"status_class"`
	EffFrom     *string `json:"eff_from"`
	EffTo       *string `json:"eff_to"`
	Method      string  `json:"method"`
}

type documentLine struct {
	Type             string  `json:"type"`
	Key              string  `json:"key"`
	NumberNorm       string  `json:"number_norm"`
	StatusClass      string  `json:"status_class"`
	EffFrom          *string `json:"eff_from"`
	EffTo            *string `json:"eff_to"`
	IsCurrent        bool    `json:"is_current"`
	ValidityConflict bool    `json:"validity_conflict"`
	TextSource       string  `json:"text_source"`
	TextComplete     bool    `json:"text_complete"`
	Baseline         bool    `json:"baseline,omitempty"`
}

type absentLine struct {
	Type   string `json:"type"`
	Key    string `json:"key"`
	Absent bool   `json:"absent"`
}

type provisionLine struct {
	Type       string        `json:"type"`
	Key        string        `json:"key"`
	Anchor     string        `json:"anchor"`
	Parent     *string       `json:"parent"`
	Ambiguous  bool          `json:"ambiguous,omitempty"`
	TextSHA256 string        `json:"text_sha256"`
	Nodes      []string      `json:"nodes"`
	Validity   []validityRow `json:"validity"`
}

type relationEnd struct {
	Key       *string `json:"key"`
	Number    string  `json:"number"`
	InRelease bool    `json:"in_release"`
	Anchor    *string `json:"anchor"`
	Exact     *bool   `json:"exact,omitempty"`
	Named     *string `json:"named,omitempty"`
	Node      *string `json:"node,omitempty"`
}

type relationIdentity struct {
	Type   string      `json:"type"`
	Kind   string      `json:"kind"`
	Method string      `json:"method"`
	Source relationEnd `json:"source"`
	Target relationEnd `json:"target"`
}

type relationLine struct {
	relationIdentity
	Count int `json:"count"`
}

type summaryRecord struct {
	Type             string        `json:"type"`
	Key              string        `json:"key"`
	Absent           bool          `json:"absent"`
	NumberNorm       string        `json:"number_norm"`
	StatusClass      string        `json:"status_class"`
	EffFrom          *string       `json:"eff_from"`
	EffTo            *string       `json:"eff_to"`
	IsCurrent        bool          `json:"is_current"`
	ValidityConflict bool          `json:"validity_conflict"`
	TextSource       string        `json:"text_source"`
	TextComplete     bool          `json:"text_complete"`
	Baseline         bool          `json:"baseline"`
	Anchor           string        `json:"anchor"`
	Parent           *string       `json:"parent"`
	Ambiguous        bool          `json:"ambiguous"`
	TextSHA256       string        `json:"text_sha256"`
	Nodes            []string      `json:"nodes"`
	Validity         []validityRow `json:"validity"`
	Kind             string        `json:"kind"`
	Method           string        `json:"method"`
	Source           relationEnd   `json:"source"`
	Target           relationEnd   `json:"target"`
	Count            int           `json:"count"`
}

func (r summaryRecord) relation() relationIdentity {
	return relationIdentity{Type: recordRelation, Kind: r.Kind, Method: r.Method, Source: r.Source, Target: r.Target}
}

type lineWriter struct {
	w   *bufio.Writer
	buf bytes.Buffer
	enc *json.Encoder
}

func newLineWriter(w io.Writer) *lineWriter {
	l := &lineWriter{w: bufio.NewWriterSize(w, 1<<16)}
	l.enc = json.NewEncoder(&l.buf)
	l.enc.SetEscapeHTML(false)
	return l
}

func encodeLine(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, fmt.Errorf("encode summary line: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func (l *lineWriter) write(value any) error {
	l.buf.Reset()
	if err := l.enc.Encode(value); err != nil {
		return fmt.Errorf("encode summary line: %w", err)
	}
	if l.buf.Len()-1 > lineLimit {
		return refuse("summary line exceeds %d bytes", lineLimit)
	}
	if _, err := l.w.Write(l.buf.Bytes()); err != nil {
		return fmt.Errorf("write summary line: %w", err)
	}
	return nil
}

func (l *lineWriter) flush() error {
	if err := l.w.Flush(); err != nil {
		return fmt.Errorf("flush summary: %w", err)
	}
	return nil
}

type lineReader struct {
	file    *os.File
	scanner *bufio.Scanner
	pending *summaryRecord
	last    string
	name    string
}

func openSummary(path string, header any) (*lineReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, refuse("open %s: %v", path, err)
	}
	r := &lineReader{file: file, scanner: bufio.NewScanner(file), name: path}
	r.scanner.Buffer(make([]byte, 0, 1<<16), lineLimit+1)
	if !r.scanner.Scan() {
		_ = file.Close()
		return nil, refuse("%s has no header: %v", path, r.scanErr())
	}
	if err = strictDecode(r.scanner.Bytes(), header); err != nil {
		_ = file.Close()
		return nil, refuse("%s header: %v", path, err)
	}
	return r, nil
}

func (r *lineReader) close() { _ = r.file.Close() }

func (r *lineReader) scanErr() error {
	if err := r.scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func strictDecode(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	return nil
}

func (r *lineReader) next() (*summaryRecord, error) {
	if r.pending != nil {
		record := r.pending
		r.pending = nil
		return record, nil
	}
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, refuse("read %s: %v", r.name, err)
		}
		return nil, nil
	}
	var record summaryRecord
	if err := strictDecode(r.scanner.Bytes(), &record); err != nil {
		return nil, refuse("decode %s line: %v", r.name, err)
	}
	if record.Type != recordDocument && record.Type != recordProvision && record.Type != recordRelation {
		return nil, refuse("%s holds an unknown record type %q", r.name, record.Type)
	}
	if record.Type < r.last {
		return nil, refuse("%s record %s follows %s", r.name, record.Type, r.last)
	}
	r.last = record.Type
	return &record, nil
}

func (r *lineReader) push(record *summaryRecord) { r.pending = record }

func (r *lineReader) section(kind string) (*summaryRecord, error) {
	for {
		record, err := r.next()
		if err != nil || record == nil {
			return nil, err
		}
		if record.Type == kind {
			return record, nil
		}
		if record.Type > kind {
			r.push(record)
			return nil, nil
		}
	}
}
