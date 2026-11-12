package ocrcheck

import (
	"bytes"
	"context"
	"image/png"
	"strconv"
	"testing"
)

func TestRenderer_DPIAndPageCount(t *testing.T) {
	for _, dpi := range []int{150, 200} {
		args := rasterArgsDPI(1, pageSize{width: 72, height: 72, cropWidth: 72, cropHeight: 72}, dpi)
		if args[5] != strconv.Itoa(dpi) || args[11] != strconv.Itoa(dpi) {
			t.Fatalf("dpi=%d args=%v", dpi, args)
		}
	}
	pages, err := CountPages(context.Background(), smallPDF())
	if err != nil || pages != 1 {
		t.Fatalf("pages=%d err=%v", pages, err)
	}
}

func TestRenderer_RealPoppler200(t *testing.T) {
	renderer := nativeSandbox(t)
	result, err := renderer.RenderDPI(context.Background(), smallPDF(), maxRequestBytes, 200)
	if err != nil || len(result.Pages) != 1 {
		t.Fatalf("render=%+v err=%v", result, err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(result.Pages[0]))
	if err != nil || config.Width != 200 || config.Height != 200 {
		t.Fatalf("config=%+v err=%v", config, err)
	}
}
