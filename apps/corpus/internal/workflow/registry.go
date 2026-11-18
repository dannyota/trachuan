package workflow

import (
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/apps/corpus/internal/stage/eval"
	"danny.vn/trachuan/apps/corpus/internal/stage/evidence"
	"danny.vn/trachuan/apps/corpus/internal/stage/expand"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"danny.vn/trachuan/internal/embedding"
)

type Registry struct{ stages []stage.Stage }

func NewRegistry(stages ...stage.Stage) Registry {
	return Registry{stages: append([]stage.Stage(nil), stages...)}
}

func (r Registry) Stages() []stage.Stage { return append([]stage.Stage(nil), r.stages...) }

func (r Registry) Names() []string {
	names := make([]string, 0, len(r.stages))
	for _, s := range r.stages {
		names = append(names, s.Name())
	}
	return names
}

func (r Registry) Following(name string) string {
	for i, s := range r.stages {
		if s.Name() == name && i+1 < len(r.stages) {
			return r.stages[i+1].Name()
		}
	}
	return ""
}

func (r Registry) Lookup(name string) (stage.Stage, bool) {
	for _, s := range r.stages {
		if s.Name() == name {
			return s, true
		}
	}
	return nil, false
}

func (r Registry) Recipes(s stage.Snapshot) map[string]string {
	recipes := make(map[string]string, len(r.stages))
	for _, st := range r.stages {
		recipes[st.Name()] = st.Recipe(s)
	}
	return recipes
}

type Deps struct {
	Env       stage.Env
	VBPL      *vbpl.Client
	Chinhphu  *chinhphu.Client
	Document  *embedding.Client
	Build     build.Config
	Waivers   extract.Waivers
	Eval      eval.Config
	Detectors []evidence.Detector
}

func Default(deps Deps) Registry {
	return NewRegistry(discover.New(deps.VBPL, discover.WithChinhphu(deps.Chinhphu)), expand.New(deps.VBPL), fetch.New(deps.VBPL).WithChinhphu(deps.Chinhphu), extract.New().WithWaivers(deps.Waivers), relate.New(), chunk.New(), embed.New(deps.Document, embedding.DocumentRecipe1()), build.New(deps.Build), eval.New(deps.Eval), evidence.New(evidence.Config{Build: deps.Eval.Build, Detectors: append(evidence.CrossSignal(), deps.Detectors...)}))
}
