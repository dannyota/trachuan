package businesskey

import "fmt"

func EncodeCitation(citation string) string {
	result := ""
	for i := 0; i < len(citation); i++ {
		b := citation[i]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '.' || b == '(' || b == ')' || b == '_' || b == '~' || b == '-' {
			result += string(b)
		} else {
			result += fmt.Sprintf("%%%02X", b)
		}
	}
	return result
}
