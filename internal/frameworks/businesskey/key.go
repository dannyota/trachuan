package businesskey

import "regexp"

const (
	keyCodePattern      = `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`
	keyVersionPattern   = `^[0-9a-z]+([.+-][0-9a-z]+)*$`
	keyCitationPattern  = `^[!-~]+( [!-~]+)*$`
	keyCitationMinBytes = 1
	keyCitationMaxBytes = 64
)

type Validator struct{ code, version, citation *regexp.Regexp }

func New() Validator {
	return Validator{regexp.MustCompile(keyCodePattern), regexp.MustCompile(keyVersionPattern), regexp.MustCompile(keyCitationPattern)}
}

func (v Validator) Valid(code, version, citation string) bool {
	return len(citation) >= keyCitationMinBytes && len(citation) <= keyCitationMaxBytes &&
		v.code.MatchString(code) && v.version.MatchString(version) && v.citation.MatchString(citation)
}

func Forms() map[string]any {
	return map[string]any{"code": keyCodePattern, "version": keyVersionPattern, "citation": keyCitationPattern, "citation_min_bytes": keyCitationMinBytes, "citation_max_bytes": keyCitationMaxBytes}
}
