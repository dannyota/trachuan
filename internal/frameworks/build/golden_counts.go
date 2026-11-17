package build

import "sort"

type GoldenCaseCount struct {
	Code         string `json:"code"`
	Version      string `json:"version"`
	Accepted     int    `json:"accepted"`
	Rejected     int    `json:"rejected"`
	Unverified   int    `json:"unverified"`
	OutOfRelease int    `json:"out_of_release"`
}

func CountGoldenCases(cases []GoldenCase) []GoldenCaseCount {
	type identity struct{ code, version string }
	byVersion := map[identity]GoldenCaseCount{}
	for _, c := range cases {
		key := identity{c.Code, c.Version}
		count := byVersion[key]
		count.Code, count.Version = c.Code, c.Version
		switch c.Adjudication {
		case "accepted":
			count.Accepted++
		case "rejected":
			count.Rejected++
		case "unverified":
			count.Unverified++
		case "out_of_release":
			count.OutOfRelease++
		}
		byVersion[key] = count
	}
	counts := make([]GoldenCaseCount, 0, len(byVersion))
	for _, count := range byVersion {
		counts = append(counts, count)
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].Code != counts[j].Code {
			return counts[i].Code < counts[j].Code
		}
		return counts[i].Version < counts[j].Version
	})
	return counts
}
