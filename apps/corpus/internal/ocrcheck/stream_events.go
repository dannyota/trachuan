package ocrcheck

import (
	"io"

	"github.com/anthropics/anthropic-sdk-go/v2/packages/ssestream"
)

// The SDK recognizes this interface to wrap its decoder while preserving tracing.
type tracedStreamBody interface {
	Unwrap() io.ReadCloser
	TraceEvents(ssestream.Decoder) ssestream.Decoder
}

type countingStreamBody struct {
	io.ReadCloser
	events *int
}

func (b countingStreamBody) Unwrap() io.ReadCloser {
	if traced, ok := b.ReadCloser.(tracedStreamBody); ok {
		return traced.Unwrap()
	}
	return b.ReadCloser
}
func (b countingStreamBody) TraceEvents(decoder ssestream.Decoder) ssestream.Decoder {
	if traced, ok := b.ReadCloser.(tracedStreamBody); ok {
		decoder = traced.TraceEvents(decoder)
	}
	return countingDecoder{Decoder: decoder, events: b.events}
}

type countingDecoder struct {
	ssestream.Decoder
	events *int
}

func (d countingDecoder) Next() bool {
	if !d.Decoder.Next() {
		return false
	}
	if d.Decoder.Event().Type != "error" {
		(*d.events)++
	}
	return true
}
func (d countingDecoder) TraceDecoded(event any) {
	if traced, ok := d.Decoder.(interface{ TraceDecoded(any) }); ok {
		traced.TraceDecoded(event)
	}
}
