package fetch

type evidenceRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type secondaryOccurrence struct {
	Name      string   `json:"name"`
	Value     *string  `json:"value"`
	Cells     []string `json:"cells"`
	Ambiguous bool     `json:"ambiguous"`
}
type secondaryAttachment struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	URL   string `json:"url"`
}
type secondaryMetadata struct {
	Version     int                   `json:"version"`
	SourceID    string                `json:"source_id"`
	SourceURL   string                `json:"source_url"`
	Occurrences []secondaryOccurrence `json:"occurrences"`
	Attachments []secondaryAttachment `json:"attachments"`
}
type secondaryDocument struct {
	Key              string        `json:"key"`
	Country          string        `json:"country"`
	Source           string        `json:"source"`
	SourceID         string        `json:"source_id"`
	Number           string        `json:"number"`
	Title            string        `json:"title"`
	DocType          string        `json:"doc_type"`
	Issuer           string        `json:"issuer"`
	IssueDate        string        `json:"raw_issue_date"`
	EffFrom          string        `json:"raw_eff_from"`
	EffTo            string        `json:"raw_eff_to"`
	StatusCode       string        `json:"status_code"`
	StatusName       string        `json:"status_name"`
	IsConsolidated   bool          `json:"is_consolidated"`
	HasContent       bool          `json:"has_content"`
	SourceURL        *string       `json:"source_url"`
	IdentityEvidence []evidenceRef `json:"identity_evidence"`
}
type secondaryDocumentEnvelope struct {
	Version  int               `json:"version"`
	Document secondaryDocument `json:"document"`
}
type secondaryFile struct {
	Source string  `json:"source"`
	Name   string  `json:"name"`
	SHA256 string  `json:"sha256"`
	Size   int64   `json:"size"`
	Path   string  `json:"path"`
	URL    *string `json:"url"`
	Role   string  `json:"role"`
}
type secondaryFiles struct {
	Version int             `json:"version"`
	Files   []secondaryFile `json:"files"`
}
type secondaryOutcome struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
type secondaryReport struct {
	Version  int                `json:"version"`
	Missing  []secondaryOutcome `json:"missing"`
	TooLarge []secondaryOutcome `json:"too_large"`
}
