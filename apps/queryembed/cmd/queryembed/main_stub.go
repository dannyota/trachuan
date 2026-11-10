//go:build !ort

package main

import (
	"fmt"
	"os"
)

func main() { _, _ = fmt.Fprintln(os.Stderr, "queryembed needs -tags ort"); os.Exit(2) }
