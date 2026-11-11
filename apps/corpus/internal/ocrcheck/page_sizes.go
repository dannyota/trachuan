package ocrcheck

import (
	"bytes"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type pageSize struct{ width, height, cropWidth, cropHeight float64 }

func parsePageSizes(data []byte) ([]pageSize, error) {
	boxLine := regexp.MustCompile(`^Page\s+(\d+)\s+(MediaBox|CropBox):\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s*$`)
	count := 0
	found := false
	boxes := map[string]map[int]pageSize{"MediaBox": {}, "CropBox": {}}
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
		match := boxLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		n, _ := strconv.Atoi(match[1])
		if n < 1 || n > maxPages {
			return nil, renderFailed
		}
		sizes := boxes[match[2]]
		if _, duplicate := sizes[n]; duplicate {
			return nil, renderFailed
		}
		var coordinates [4]float64
		for i := range coordinates {
			v, err := strconv.ParseFloat(match[i+3], 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, renderFailed
			}
			coordinates[i] = v
		}
		w, h := coordinates[2]-coordinates[0], coordinates[3]-coordinates[1]
		if w <= 0 || h <= 0 || math.IsInf(w, 0) || math.IsInf(h, 0) {
			return nil, renderFailed
		}
		sizes[n] = pageSize{width: w, height: h}
	}
	if !found || len(boxes["MediaBox"]) != count || len(boxes["CropBox"]) != count {
		return nil, renderFailed
	}
	pages := make([]pageSize, count)
	for i := range pages {
		media, mediaOK := boxes["MediaBox"][i+1]
		crop, cropOK := boxes["CropBox"][i+1]
		if !mediaOK || !cropOK {
			return nil, renderFailed
		}
		// Bound both interpretations before Poppler allocates the CropBox raster.
		pages[i] = pageSize{width: math.Max(media.width, crop.width), height: math.Max(media.height, crop.height), cropWidth: crop.width, cropHeight: crop.height}
	}
	return pages, nil
}

func rasterArgs(page int, size pageSize) []string {
	n := strconv.Itoa(page)
	dpi := 150.0
	extent := math.Max(size.width, size.height)
	if math.Ceil(extent*dpi/72) > maxImageSide {
		dpi = float64(maxImageSide) * 72 / extent
		for math.Ceil(extent*dpi/72) > maxImageSide {
			dpi = math.Nextafter(dpi, 0)
		}
	}
	// An explicit cap also bounds Poppler's allocation independently of page units.
	cropPixels := math.Ceil(math.Max(size.cropWidth, size.cropHeight) * dpi / 72)
	cap := int(math.Max(1, math.Min(maxImageSide, cropPixels)))
	return []string{"-f", n, "-l", n, "-r", strconv.FormatFloat(dpi, 'g', -1, 64), "-gray", "-png", "-singlefile", "-cropbox", "-scale-to", strconv.Itoa(cap), "input.pdf", "page"}
}

type metadataBuffer struct{ bytes.Buffer }

func (b *metadataBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 1_000_000 {
		return 0, renderLimit
	}
	return b.Buffer.Write(data)
}
