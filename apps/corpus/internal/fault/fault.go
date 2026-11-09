//go:build !faultinject

// Package fault provides fault injection hooks.
package fault

// Hit is a no-op when fault injection is disabled.
func Hit(point string, n int) {}
