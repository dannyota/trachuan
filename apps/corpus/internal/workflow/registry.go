// Package workflow coordinates corpus release stages.
package workflow

import (
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage/discover"
	"github.com/dannyota/traquydinh/apps/corpus/internal/vbpl"
)

// Registry holds the implemented stages in execution order.
type Registry struct{ stages []stage.Stage }

// NewRegistry constructs a registry in stage order.
func NewRegistry(stages ...stage.Stage) Registry {
	return Registry{stages: append([]stage.Stage(nil), stages...)}
}

// Stages returns the registered stages in order.
func (r Registry) Stages() []stage.Stage { return append([]stage.Stage(nil), r.stages...) }

// Names returns the registered stage names in order.
func (r Registry) Names() []string {
	names := make([]string, 0, len(r.stages))
	for _, s := range r.stages {
		names = append(names, s.Name())
	}
	return names
}

// Following returns the name of the next stage, if any.
func (r Registry) Following(name string) string {
	for i, s := range r.stages {
		if s.Name() == name && i+1 < len(r.stages) {
			return r.stages[i+1].Name()
		}
	}
	return ""
}

// Lookup finds a stage by name.
func (r Registry) Lookup(name string) (stage.Stage, bool) {
	for _, s := range r.stages {
		if s.Name() == name {
			return s, true
		}
	}
	return nil, false
}

// Recipes computes the registered recipes for the frozen snapshot.
func (r Registry) Recipes(s stage.Snapshot) map[string]string {
	recipes := make(map[string]string, len(r.stages))
	for _, st := range r.stages {
		recipes[st.Name()] = st.Recipe(s)
	}
	return recipes
}

// Deps holds dependencies for the default stages.
type Deps struct {
	Env  stage.Env
	VBPL *vbpl.Client
}

// Default returns the stages compiled into the worker.
func Default(deps Deps) Registry { return NewRegistry(discover.New(deps.VBPL)) }
