package serve

import "slices"

func (e *Engine) Release() string { return e.release }

func IsHandle(raw string) bool {
	_, err := parseHandle(raw)
	return err == nil
}

func (e *Engine) FrameworkCodes() []string {
	codes := make([]string, 0, len(e.versions))
	for _, v := range e.versions {
		codes = append(codes, v.info.Code)
	}
	slices.Sort(codes)
	return slices.Compact(codes)
}
