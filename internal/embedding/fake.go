package embedding

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strings"
)

// TestRecipeHash identifies the recipe reserved for loopback tests.
const TestRecipeHash = "763411dc501717a2d9333cec5f10eb39505bd338b3c69991362369c292eed193"

// TestRecipe returns the fixed test-only embedding recipe.
func TestRecipe() Recipe {
	return Recipe{Model: "trachuan-test-only", Revision: "fake", ResolvedRevision: "fake", TokenizerRevision: "fake", TokenizerSHA256: strings.Repeat("0", 64), WeightsSHA256: strings.Repeat("0", 64), Dimensions: 1024, Pooling: "cls", Normalization: "l2-float32", Input: "raw", Runtime: "fake", Backend: "fake", Precision: "fake", TokenCap: 256, SpecialTokens: true, Language: "fake", TokenizerLibrary: "fake", RuntimeLibrary: "fake"}
}

// FakeVector returns a deterministic normalized vector for test text.
func FakeVector(text string) []float32 {
	values := make([]float32, TestRecipe().Dimensions)
	seed := sha256.Sum256([]byte(text))
	var sum float64
	for i := range values {
		var input [36]byte
		copy(input[:], seed[:])
		binary.BigEndian.PutUint32(input[32:], uint32(i))
		hash := sha256.Sum256(input[:])
		value := float64(binary.BigEndian.Uint32(hash[:4]))/float64(^uint32(0))*2 - 1
		values[i] = float32(value)
		sum += float64(values[i]) * float64(values[i])
	}
	norm := math.Sqrt(sum)
	for i := range values {
		values[i] = float32(float64(values[i]) / norm)
	}
	return values
}
