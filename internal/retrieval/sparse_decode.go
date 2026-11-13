package retrieval

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"danny.vn/trachuan/internal/retrieval/lexical"
)

func decodeSparse(text string, dimensions int) (lexical.Vector, float64, error) {
	body, suffix, ok := strings.Cut(text, "}/")
	d, err := strconv.Atoi(suffix)
	if !ok || err != nil || d != dimensions || !strings.HasPrefix(body, "{") {
		return nil, 0, errors.New("invalid sparse dimensions or encoding")
	}
	body = body[1:]
	var vector lexical.Vector
	var sum float64
	var previous int
	for body != "" {
		entry, rest, more := strings.Cut(body, ",")
		index, value, found := strings.Cut(entry, ":")
		i, indexErr := strconv.Atoi(index)
		v, valueErr := strconv.ParseFloat(value, 32)
		if !found || indexErr != nil || valueErr != nil || i <= previous || i > d || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) || (more && rest == "") {
			return nil, 0, errors.New("invalid sparse entry")
		}
		vector = append(vector, lexical.Entry{Index: int32(i), Value: float32(v)})
		sum += v
		previous, body = i, rest
	}
	u := math.Ldexp(1, -24)
	k := float64(max(len(vector)-1, 0))
	if k*u >= 1 || sum > math.MaxFloat32 {
		return nil, 0, errors.New("sparse sum exceeds exactness bound")
	}
	bound := 1.01 * 2 * (k * u / (1 - k*u)) * sum
	if sum+bound > math.MaxFloat32 {
		return nil, 0, errors.New("sparse sum risks float32 overflow")
	}
	return vector, bound, nil
}
