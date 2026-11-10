package httpapi

import "math"

func validVector(v []float32, dimensions int) bool {
	if len(v) != dimensions || dimensions <= 0 {
		return false
	}
	sum := 0.0
	for _, x := range v {
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return false
		}
		sum += f * f
	}
	return math.Abs(math.Sqrt(sum)-1) <= 0.001
}
