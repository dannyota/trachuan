package relate

import "danny.vn/trachuan/internal/frameworks/registry"

func ResolveVersion(reg *registry.Registry, code, label string) (string, string, bool) {
	return resolveVersion(reg, code, label)
}

func NormalizeCitation(reg *registry.Registry, key Key) string {
	return norm(lookupCitation(reg, key))
}
