// Package fetch downloads VBPL documents into content-addressed bundles.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/sync/errgroup"
)

// Stage fetches documents and their attachments.
type Stage struct {
	client       *vbpl.Client
	chinhphu     *chinhphu.Client
	cache        *filecache.Cache
	fileLimit    int64
	loadBaseline func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error)
}

// New constructs a fetch stage.
func New(client *vbpl.Client) *Stage { return &Stage{client: client, fileLimit: 50_000_000} }

// WithChinhphu sets the secondary source client.
func (s *Stage) WithChinhphu(client *chinhphu.Client) *Stage { s.chinhphu = client; return s }

// WithCache sets the optional attachment cache.
func (s *Stage) WithCache(cache *filecache.Cache) *Stage { s.cache = cache; return s }

// WithFileLimit sets the attachment size limit in bytes.
func (s *Stage) WithFileLimit(limit int64) *Stage { s.fileLimit = limit; return s }

// Name returns the stage name.
func (*Stage) Name() string { return "fetch" }

// Recipe returns the fetch recipe hash.
func (*Stage) Recipe(stage.Snapshot) string {
	return stage.RecipeHash("fetch", "vbpl-fetch/2", "chinhphu-fetch/2", "json:encoding/json:compact:no-newline",
		"metadata:version,source_id,source_url,occurrences,attachments;occurrence:name,value,cells,ambiguous;attachment:name,label,url",
		"document:version,document;fields:key,country,source,source_id,number,title,doc_type,issuer,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,is_consolidated,has_content,source_url,identity_evidence",
		"files:version,files;file:source,name,sha256,size,path,url,role;report:version,missing,too_large;outcome:name,url;evidence:path,sha256")
}

// Plan returns the expected documents and their skip reasons.
func (*Stage) Plan(ctx context.Context, env stage.Env, r stage.Run) ([]stage.Item, error) {
	expected, err := discover.ExpectedSet(ctx, env.DB, r, env.Artifacts)
	if err != nil {
		return nil, fmt.Errorf("load expected documents: %w", err)
	}
	items := make([]stage.Item, 0, len(expected))
	for _, e := range expected {
		items = append(items, stage.Item{Key: e.Key, Skip: e.Skip})
	}
	return items, nil
}

// Process fetches the source responses and publishes a bundle.
func (s *Stage) Process(ctx context.Context, env stage.Env, r stage.Run, item stage.Item) (stage.Output, error) {
	var decision readDecision
	return s.processItem(ctx, env, r, item, &decision)
}

func (s *Stage) processItem(ctx context.Context, env stage.Env, r stage.Run, item stage.Item, result *readDecision) (output stage.Output, err error) {
	if strings.HasPrefix(item.Key, "vn:chinhphu:") {
		out, processErr := s.processChinhphu(ctx, env, r, item)
		out.Sources = []string{"chinhphu"}
		return out, processErr
	}
	defer func() {
		if s.client != nil && strings.HasPrefix(item.Key, "vn:vbpl:") && len(item.Key) > len("vn:vbpl:") {
			output.Sources = []string{"vbpl"}
		}
	}()
	const prefix = "vn:vbpl:"
	if !strings.HasPrefix(item.Key, prefix) || len(item.Key) == len(prefix) {
		return stage.Output{}, fmt.Errorf("invalid document key %q", item.Key)
	}
	if s.client == nil {
		return stage.Output{}, fmt.Errorf("VBPL client is required")
	}
	id := strings.TrimPrefix(item.Key, prefix)
	var detail, tree, diagram json.RawMessage
	var decoded vbpl.Detail
	var files []vbpl.File
	var errs [4]error
	var times [3]time.Time
	now := env.Now
	if now == nil {
		now = time.Now
	}
	group, requestCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		detail, decoded, errs[0] = s.client.Detail(requestCtx, id)
		times[0] = now().UTC().Truncate(time.Microsecond)
		return nil
	})
	group.Go(func() error {
		diagram, errs[2] = s.client.Diagram(requestCtx, id)
		times[2] = now().UTC().Truncate(time.Microsecond)
		return nil
	})
	_ = group.Wait()
	if errs[0] != nil {
		return apiError(errs[0], id, true)
	}
	detail, err = removeViewCount(detail)
	if err != nil {
		return apiError(err, id, true)
	}
	if decoded.ID != id {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "detail_id_mismatch", Detail: fmt.Sprintf("requested %q, received %q", id, decoded.ID)}}}, nil
	}
	if errs[2] != nil {
		return apiError(errs[2], id, false)
	}
	loader := s.loadBaseline
	if loader == nil {
		loader = loadBaseline
	}
	base, invalid, err := loader(ctx, env, r, item)
	if err != nil {
		return stage.Output{}, err
	}
	if invalid {
		base = &baseline{invalid: true}
	}
	if base != nil && !base.invalid && base.index == nil {
		if err = base.indexOrigins(ctx); err != nil {
			return stage.Output{}, err
		}
	}
	*result = evaluateBaseline(detail, base, now(), reuseAgeLimit)
	wouldReuse := result.reason == "reused"
	if controlSample(item.Key, r.Rel) {
		result.reason = "control"
	}
	if result.reason == "reused" {
		out, reuseErr := s.publishReuse(ctx, env, r, result.baseline, detail, diagram, times)
		if reuseErr == nil {
			slog.Info("fetch decision", "run", r.Country+"/"+r.Rel, "key", item.Key, "decision", result.reason)
			out.Message = result.message()
			return out, nil
		}
		if !errors.Is(reuseErr, errInvalidBaseline) {
			return stage.Output{}, reuseErr
		}
		slog.Warn("invalid fetch baseline", "run", r.Country+"/"+r.Rel, "key", item.Key, "reason", reuseErr)
		result.reason, result.baseline = "invalid_baseline", nil
	}
	group, requestCtx = errgroup.WithContext(ctx)
	group.Go(func() error {
		tree, errs[1] = s.client.RawTree(requestCtx, id)
		times[1] = now().UTC().Truncate(time.Microsecond)
		return nil
	})
	group.Go(func() error { files, errs[3] = s.client.Files(requestCtx, id); return nil })
	_ = group.Wait()
	for _, e := range []error{errs[1], errs[3]} {
		if e != nil {
			return apiError(e, id, false)
		}
	}
	var observations []stage.FetchObservation
	for i, payload := range []struct {
		endpoint, path string
		data           []byte
	}{{"doc/detail", "detail.json", detail}, {"doc/provision/tree", "tree.json", tree}, {"doc/diagram", "diagram.json", diagram}} {
		key, _ := json.Marshal([3]any{payload.endpoint, nil, nil})
		observations = append(observations, stage.FetchObservation{ResourceKey: string(key), Source: "vbpl", Path: payload.path, ContentSHA256: fmt.Sprintf("%x", sha256.Sum256(payload.data)), Size: int64(len(payload.data)), Acquisition: "network", FetchedAt: &times[i]})
	}
	publish := func(b *baseline, reason string) (artifact.Result, error) {
		observations = observations[:3]
		return env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dir string) error {
			return s.writeBundleWithBaseline(ctx, dir, id, detail, tree, diagram, files, now, &observations, b, reason, result.expired)
		})
	}
	art, err := publish(result.baseline, result.reason)
	if errors.Is(err, errInvalidBaseline) {
		slog.Warn("invalid fetch baseline", "run", r.Country+"/"+r.Rel, "key", item.Key, "reason", err)
		result.reason = "invalid_baseline"
		art, err = publish(nil, result.reason)
	}
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish fetch artifact: %w", err)
	}
	if result.reason == "control" && wouldReuse {
		dir := env.Artifacts.Abs(art.Path)
		filesJSON, readErr := os.ReadFile(filepath.Join(dir, "files.json"))
		if readErr != nil {
			return stage.Output{}, fmt.Errorf("read control files: %w", readErr)
		}
		reportJSON, readErr := os.ReadFile(filepath.Join(dir, "report.json"))
		if readErr != nil {
			return stage.Output{}, fmt.Errorf("read control report: %w", readErr)
		}
		var differs []string
		result.outcome, differs = compareControl(base, tree, filesJSON, reportJSON)
		if result.outcome == "changed" {
			slog.Warn("fetch control changed", "document_key", item.Key, "files", differs)
		}
	}
	slog.Info("fetch decision", "run", r.Country+"/"+r.Rel, "key", item.Key, "decision", result.reason)

	return stage.Output{Artifact: &art, State: "done", Message: result.message(), Observations: observations}, nil
}
func apiError(err error, id string, detail bool) (stage.Output, error) {
	if errors.Is(err, vbpl.ErrTooLarge) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "response_too_large", Detail: id}}}, nil
	}
	if detail && errors.Is(err, vbpl.ErrNotFound) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "not_found", Detail: id}}}, nil
	}
	wrapped := fmt.Errorf("fetch document %q: %w", id, err)
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &typeErr) {
		return stage.Output{}, stage.SourceData(wrapped)
	}
	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return stage.Output{}, stage.Retryable(wrapped)
	}
	var failure *vbpl.Failure
	if errors.As(err, &failure) {
		cause := stage.GapCause{Source: "vbpl", Origin: stage.Origin(failure.Origin), Cause: stage.Cause(failure.Cause), Endpoint: failure.Endpoint}
		if failure.HTTPStatus != 0 {
			status := int32(failure.HTTPStatus)
			cause.HTTPStatus = &status
		}
		if cause.Cause == stage.CauseHTTP5xx {
			return unavailableNow(wrapped, cause), nil
		}
		return stage.Output{}, &stage.RetryableError{Err: wrapped, GapKind: "source_unavailable", Cause: cause}
	}
	var status *vbpl.StatusError
	if errors.As(err, &status) && (status.Code == 429 || status.Code >= 500 && status.Code <= 599) {
		code := int32(status.Code)
		cause := stage.GapCause{Source: "vbpl", Origin: stage.OriginGateway, Cause: stage.CauseHTTP5xx, Endpoint: status.Endpoint, HTTPStatus: &code}
		if code == 429 {
			cause.Cause = stage.CauseHTTP429
			return stage.Output{}, &stage.RetryableError{Err: wrapped, GapKind: "source_unavailable", Cause: cause}
		}
		return unavailableNow(wrapped, cause), nil
	}

	return stage.Output{}, stage.Retryable(wrapped)
}

// unavailableNow turns a gateway 5xx that survived the client retries into a
// source_unavailable gap in this attempt, without a batch retry, keeping its
// structured cause so a later start can retry it.
func unavailableNow(err error, c stage.GapCause) stage.Output {
	detail := []rune(err.Error())
	if len(detail) > 200 {
		detail = detail[:200]
	}
	return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "source_unavailable", Detail: string(detail), Source: c.Source, Origin: c.Origin, Cause: c.Cause, Endpoint: c.Endpoint, HTTPStatus: c.HTTPStatus, EvidenceRef: c.EvidenceRef}}}
}
