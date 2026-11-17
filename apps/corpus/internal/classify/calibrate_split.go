package classify

import "sort"

func splitCalibrationSet(s CalibrationSet) (CalibrationSet, CalibrationSet) {
	labels := append(s.Labels.Articles[:0:0], s.Labels.Articles...)
	sort.Slice(labels, func(i, j int) bool {
		a, b := labels[i].Key+"/"+labels[i].Anchor, labels[j].Key+"/"+labels[j].Anchor
		ha, hb := digest([]byte(a)), digest([]byte(b))
		if ha == hb {
			return a < b
		}
		return ha < hb
	})
	a, b := s, s
	a.Labels.Articles = labels[:len(labels)/2]
	b.Labels.Articles = labels[len(labels)/2:]
	return a, b
}
