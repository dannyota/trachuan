package expand

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

// VerifyAcquisition checks the detail identity and frozen relation projection.
// The caller has already verified the whole bundle and its recorded hash.
func (*Stage) VerifyAcquisition(_ context.Context, env stage.Env, r stage.Run, item stage.Item, art artifact.Result) error {
	dir := env.Artifacts.Abs(art.Path)
	detail, err := os.ReadFile(filepath.Join(dir, "detail.json"))
	if err != nil {
		return fmt.Errorf("read expand detail: %w", err)
	}
	var d struct{ Data struct{ ID string } }
	if err = json.Unmarshal(detail, &d); err != nil {
		return fmt.Errorf("decode expand detail: %w", err)
	}
	if d.Data.ID == "" || r.Country+":vbpl:"+d.Data.ID != item.Key {
		return fmt.Errorf("expand detail identity mismatch")
	}
	diagram, err := os.ReadFile(filepath.Join(dir, "diagram.json"))
	if err != nil {
		return fmt.Errorf("read expand diagram: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "observation.json"))
	if err != nil {
		return fmt.Errorf("read expand observation: %w", err)
	}
	var observed Observation
	if err = json.Unmarshal(data, &observed); err != nil {
		return fmt.Errorf("decode expand observation: %w", err)
	}
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return fmt.Errorf("parse frozen source list: %w", err)
	}
	want, err := Qualify(item.Key, detail, diagram, list.RelationCodes)
	if err != nil {
		return fmt.Errorf("qualify expand acquisition: %w", err)
	}
	if !reflect.DeepEqual(want, observed) {
		return fmt.Errorf("expand observation differs from frozen projection")
	}
	return nil
}
