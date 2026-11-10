package testpg

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"regexp"
	"time"
)

func releaseCandidates(name string) []string {
	hash := sha256.Sum256([]byte(name))
	start := int(binary.BigEndian.Uint32(hash[:4]) % 365)
	first := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	candidates := make([]string, 0, 36500)
	for day := 0; day < 365; day++ {
		date := first.AddDate(0, 0, (start+day)%365).Format("20060102")
		candidates = append(candidates, date)
		for suffix := 1; suffix <= 99; suffix++ {
			candidates = append(candidates, fmt.Sprintf("%s_%d", date, suffix))
		}
	}
	return candidates
}

func validRunKey(key string) bool {
	if !regexp.MustCompile(`^vn/2099[0-9]{4}(_[0-9]{1,2})?$`).MatchString(key) {
		return false
	}
	_, err := time.Parse("20060102", key[3:11])
	return err == nil
}
