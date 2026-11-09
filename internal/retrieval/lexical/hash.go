package lexical

import "hash/fnv"

// Dimension maps a token to its one-based hashed dimension.
func Dimension(token string, recipe Recipe) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(token))
	return int32(h.Sum32()%uint32(recipe.Dimensions) + 1)
}
