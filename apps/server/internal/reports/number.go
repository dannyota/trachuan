package reports

import (
	"context"
	"strconv"
	"strings"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

func validateNumber(ctx context.Context, corpus Corpus, kind, value string, now time.Time) (string, error) {
	if !boundedString(value, retrieval.ReadNumberMax) {
		return "", ErrInvalidRequest
	}
	number := retrieval.NormalizeNumber(value)
	if number == "" {
		return "", ErrInvalidRequest
	}
	if kind == "broken_citation" {
		exists, err := corpus.HasNumber(ctx, number)
		if err != nil {
			return "", corpusFailure(err)
		}
		if !exists {
			return "", ErrInvalidRequest
		}
		return number, nil
	}
	parts := strings.SplitN(number, "/", 3)
	if len(parts) != 3 || !decimal(parts[0], 1, 4) || !decimal(parts[1], 4, 4) {
		return "", ErrInvalidRequest
	}
	year, _ := strconv.Atoi(parts[1])
	if year < 1945 || year > now.UTC().Year() {
		return "", ErrInvalidRequest
	}
	segments := strings.Split(strings.ReplaceAll(parts[2], "/", "-"), "-")
	if len(segments) < 1 || len(segments) > 4 {
		return "", ErrInvalidRequest
	}
	for _, segment := range segments {
		known, err := corpus.HasNumberSegment(ctx, segment)
		if err != nil {
			return "", corpusFailure(err)
		}
		if !known || segment == "" {
			return "", ErrInvalidRequest
		}
	}
	return number, nil
}

func decimal(value string, minLength, maxLength int) bool {
	if len(value) < minLength || len(value) > maxLength {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
