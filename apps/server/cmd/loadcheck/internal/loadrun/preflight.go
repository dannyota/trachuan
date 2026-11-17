package loadrun

import "math"

func requiredQueries(c Config, summaries int) int {
	total := 0
	for _, phase := range c.Phases {
		var seconds float64
		switch phase {
		case "A":
			seconds = c.DurationA.Seconds()
		case "B":
			seconds = float64(summaries) / 8 * 1.25
		case "C":
			seconds = c.DurationC.Seconds()
		case "D":
			seconds = c.DurationD.Seconds()
		}
		total += int(math.Ceil(seconds * 2))
	}
	return total
}
