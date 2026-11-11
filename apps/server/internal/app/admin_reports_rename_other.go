//go:build !darwin && !linux

package app

import "errors"

func renameReportDirectory(_, _ string) error {
	return errors.New("atomic directory rename unavailable")
}
