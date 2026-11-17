package app

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"danny.vn/trachuan/internal/corpusdb/frameworks"
)

func runFrameworksSchema(args []string, stdout io.Writer) error {
	if len(args) != 2 || args[0] != "--version" {
		return errors.New("invalid frameworks-schema arguments")
	}
	version, err := strconv.Atoi(args[1])
	if err != nil {
		return errors.New("invalid frameworks schema version")
	}
	sql, err := frameworks.ExecutableSQL(version)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(stdout, sql); err != nil {
		return fmt.Errorf("write frameworks schema: %w", err)
	}
	return nil
}
