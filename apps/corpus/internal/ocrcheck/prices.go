package ocrcheck

import "fmt"

const maxOutputTokens int64 = 32000

// Usage records token totals and the charge in millionths of a US dollar.
// No caching is requested. Unexpected cache usage blocks further work because
// the verified price table covers uncached requests only.
type Usage struct {
	PDFPages            int          `json:"pdf_pages,omitempty"`
	Rendering           *RenderStats `json:"rendering,omitempty"`
	Final               bool         `json:"final_usage"`
	CountedInput        int64        `json:"counted_input_tokens,omitempty"`
	MaxOutput           int64        `json:"max_output_tokens,omitempty"`
	ReservationMicroUSD int64        `json:"reservation_micro_usd,omitempty"`
	APIStatus           int          `json:"api_status,omitempty"`
	RequestID           string       `json:"request_id,omitempty"`
	Input               int64        `json:"input_tokens"`
	Output              int64        `json:"output_tokens"`
	CacheRead           int64        `json:"cache_read_input_tokens"`
	CacheCreation       int64        `json:"cache_creation_input_tokens"`
	CostMicroUSD        int64        `json:"cost_micro_usd"`
}

type price struct{ input, output int64 }

func modelPrice(model string, batch bool) (price, error) {
	// USD per million tokens, equivalently microUSD per token. Verified 2026-11-11:
	// https://platform.claude.com/docs/en/about-claude/pricing
	var p price
	switch model {
	case "claude-sonnet-5-5":
		p = price{2, 10}
	case "claude-opus-5-5":
		p = price{4, 20}
	default:
		return p, fmt.Errorf("unsupported OCR checker model")
	}
	// Message Batches cost half the direct price:
	// https://platform.claude.com/docs/en/build-with-claude/batch-processing
	if batch {
		p.input /= 2
		p.output /= 2
	}
	return p, nil
}
func (p price) cost(u Usage) int64 {
	return u.Input*p.input + u.Output*p.output
}

func (u Usage) priceError() error {
	if u.CacheRead != 0 || u.CacheCreation != 0 {
		return fmt.Errorf("unexpected cache usage: charge is incomplete; verify cache prices before further OCR work")
	}
	return nil
}

func (u Usage) charge() int64 {
	if !u.Final && u.ReservationMicroUSD > u.CostMicroUSD {
		return u.ReservationMicroUSD
	}
	return u.CostMicroUSD
}
func (u Usage) withReservation(intent Usage) Usage {
	u.PDFPages = intent.PDFPages
	u.Rendering = intent.Rendering
	u.CountedInput = intent.CountedInput
	u.MaxOutput = intent.MaxOutput
	u.ReservationMicroUSD = intent.ReservationMicroUSD
	return u
}
