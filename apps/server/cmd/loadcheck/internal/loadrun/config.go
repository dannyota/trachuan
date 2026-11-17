package loadrun

import "time"

type Config struct {
	Pages, API, PublicURL, Google, Out, Activate string
	Queries, Emails, Sessions, Phases            []string
	DurationA, DurationC, DurationD              time.Duration
}

type Record struct {
	Phase      string    `json:"phase"`
	Stream     string    `json:"stream"`
	Key        string    `json:"key,omitempty"`
	Pass       int       `json:"pass,omitempty"`
	Start      time.Time `json:"start"`
	DurationMS float64   `json:"duration_ms"`
	Status     int       `json:"status"`
	Retries    int       `json:"retries"`
	Aborted    bool      `json:"aborted"`
}

type Stream struct {
	Status                 map[int]int `json:"status"`
	Count, Retries, Aborts int
	P50MS, P95MS, P99MS    float64
}
type Report struct {
	Phases      map[string]map[string]Stream `json:"phases"`
	Gates       map[string]string            `json:"gates"`
	Activations []time.Time                  `json:"activations"`
}
