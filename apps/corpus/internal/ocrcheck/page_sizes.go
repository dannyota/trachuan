package ocrcheck

import (
	"bytes"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type pageSize struct{ width, height float64 }

func parsePageSizes(data []byte) ([]pageSize, error) {
	sizeLine := regexp.MustCompile(`^Page\s+(\d+)\s+size:\s+(\S+)\s+x\s+(\S+)\s+pts(?:\s|$)`)
	count := 0
	found := false
	sizes := map[int]pageSize{}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			if found {
				return nil, renderFailed
			}
			found = true
			var err error
			count, err = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
			if err != nil || count < 1 {
				return nil, renderFailed
			}
			if count > maxPages {
				return nil, renderLimit
			}
		}
		match := sizeLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		n, _ := strconv.Atoi(match[1])
		if n < 1 || n > maxPages {
			return nil, renderFailed
		}
		if _, duplicate := sizes[n]; duplicate {
			return nil, renderFailed
		}
		w, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			return nil, renderFailed
		}
		h, err := strconv.ParseFloat(match[3], 64)
		if err != nil {
			return nil, renderFailed
		}
		if w <= 0 || h <= 0 || math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) {
			return nil, renderFailed
		}
		sizes[n] = pageSize{w, h}
	}
	if !found || len(sizes) != count {
		return nil, renderFailed
	}
	pages := make([]pageSize, count)
	for i := range pages {
		size, ok := sizes[i+1]
		if !ok {
			return nil, renderFailed
		}
		pages[i] = size
	}
	return pages, nil
}

func rasterArgs(page int, size pageSize) []string {
	n := strconv.Itoa(page)
	args := []string{"-f", n, "-l", n, "-r", "150", "-gray", "-png", "-singlefile"}
	if math.Ceil(size.width*150/72) > maxImageSide || math.Ceil(size.height*150/72) > maxImageSide {
		args = append(args, "-scale-to", "3000")
	}
	return append(args, "input.pdf", "page")
}

type metadataBuffer struct{ bytes.Buffer }

func (b *metadataBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 1_000_000 {
		return 0, renderLimit
	}
	return b.Buffer.Write(data)
}
