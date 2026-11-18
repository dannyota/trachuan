package observe

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func readConnections(path string) ([]sample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("open connections failed")
	}
	lines := strings.Split(string(data), "\n")
	lines = lines[:len(lines)-1]
	var result []sample
	for i, text := range lines {
		line := i + 1
		fields := strings.Fields(text)
		invalid := fmt.Errorf("invalid connections line %d", line)
		if len(fields) != 2 {
			return nil, invalid
		}
		at, err := time.Parse(time.RFC3339Nano, fields[0])
		if _, offset := at.Zone(); err != nil || offset != 0 {
			return nil, invalid
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil || count < 0 {
			return nil, invalid
		}
		result = append(result, sample{Time: at, Connections: &count})
	}
	return result, nil
}
