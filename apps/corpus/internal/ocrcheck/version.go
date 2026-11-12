package ocrcheck

import "fmt"

type versionSettings struct {
	dpi        int
	mediaType  string
	directOnly bool
}

func (o Options) version() (versionSettings, error) {
	n := o.CheckVersion
	if n == 0 {
		n = 2
	}
	versions := map[int]versionSettings{
		2: {150, "image/png", false},
		3: {200, "image/jpeg", true},
		4: {150, "image/jpeg", true},
	}
	v, ok := versions[n]
	if !ok {
		return v, fmt.Errorf("--check-version must be 2, 3 or 4")
	}
	return v, nil
}

// ValidateVersion rejects unknown versions and direct-only batch requests.
func (o Options) ValidateVersion() error {
	v, err := o.version()
	if err != nil {
		return err
	}
	if o.Batch && v.directOnly {
		return fmt.Errorf("--batch requires --check-version 2")
	}
	return nil
}
