package evidence

import "danny.vn/trachuan/apps/corpus/internal/detector"

func specOf(id string) detector.Detector {
	spec, ok := detector.Lookup(id)
	if !ok {
		panic("detector is not registered: " + id)
	}
	return spec
}

func CrossSignal() []Detector {
	return []Detector{rawFieldDetector{}, partialDetector{}, claimDetector{}, waivedDetector{}}
}
