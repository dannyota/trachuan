//go:build !linux && !darwin

package app

import (
	"errors"
	"os"
)

func duplicateAdminSessionFile(*os.File) (*os.File, error) {
	return nil, errors.New("session file descriptor duplication unavailable")
}
