package authmail

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type Capture struct{ dir string }

func NewCapture(dir string) (*Capture, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create mail capture directory: %w", err)
	}
	return &Capture{dir: dir}, nil
}
func (s *Capture) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dir, "message-*.json")
	if err != nil {
		return fmt.Errorf("create capture message: %w", err)
	}
	name := file.Name()
	if err = json.NewEncoder(file).Encode(msg); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write capture message: %w", err)
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close capture message: %w", err)
	}
	return nil
}
