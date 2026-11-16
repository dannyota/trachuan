package classify

import (
	"fmt"
	"os"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

func NewClient() (batchapi.SDK, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return batchapi.SDK{}, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	return batchapi.SDK{Client: anthropic.NewClient(option.WithoutEnvironmentDefaults(), option.WithAPIKey(key), option.WithMaxRetries(0), option.WithRequestTimeout(10*time.Minute))}, nil
}
