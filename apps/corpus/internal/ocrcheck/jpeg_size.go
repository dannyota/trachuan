package ocrcheck

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// jpegRequestSize sums the prepared blocks without marshaling their payloads.
// Only the small envelope and empty block shapes pass through the SDK encoder.
func jpegRequestSize(r Request) (int, error) {
	p := messageParams(r)
	content := p.Messages[0].OfUser.Content
	p.Messages = []anthropic.MessageParam{anthropic.NewUserMessage([]anthropic.ContentBlockParamUnion{}...)}
	envelope, err := json.Marshal(p)
	if err != nil {
		return 0, fmt.Errorf("size OCR request envelope: %w", err)
	}
	total := len(envelope) + len(`,"stream":true`)
	for i, b := range content {
		var empty anthropic.ContentBlockParamUnion
		switch {
		case b.OfText != nil:
			empty = anthropic.NewTextBlock("")
			total += jsonStringSize(b.OfText.Text) - 2
		case b.OfImage != nil && b.OfImage.Source.OfBase64 != nil:
			source := b.OfImage.Source.OfBase64
			empty = anthropic.NewImageBlockBase64(string(source.MediaType), "")
			// Base64 uses no characters that JSON escapes.
			total += len(source.Data)
		default:
			return 0, fmt.Errorf("unsupported OCR request block")
		}
		shape, encodeErr := json.Marshal(empty)
		if encodeErr != nil {
			return 0, fmt.Errorf("size OCR request block: %w", encodeErr)
		}
		total += len(shape)
		if i > 0 {
			total++
		}
	}
	return total, nil
}

// The SDK's Go 1.24 JSON shim escapes invalid UTF-8 as six-byte replacements.
func jsonStringSize(s string) int {
	n := 2
	for len(s) > 0 {
		c := s[0]
		if c < utf8.RuneSelf {
			switch c {
			case '"', '\\', '\b', '\f', '\n', '\r', '\t':
				n += 2
			case '<', '>', '&':
				n += 6
			default:
				if c < 0x20 {
					n += 6
				} else {
					n++
				}
			}
			s = s[1:]
			continue
		}
		r, size := utf8.DecodeRuneInString(s)
		if (r == utf8.RuneError && size == 1) || r == '\u2028' || r == '\u2029' {
			n += 6
		} else {
			n += size
		}
		s = s[size:]
	}
	return n
}
