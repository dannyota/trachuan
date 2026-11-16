package classify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

const creatorQuietWindow = 24*time.Hour + 10*time.Minute

type creatorGuard struct{ file *os.File }

func (r *Runner) lockCreator(id string) (*creatorGuard, error) {
	if !validID(id) || r.IntentDir == "" {
		return nil, fmt.Errorf("creator requires a submission identity and intent directory")
	}
	dir := filepath.Join(r.IntentDir, "submissions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create creator lease directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, id+".creator.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open creator lease: %w", err)
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("submission creator is still live")
		}
		return nil, fmt.Errorf("lock submission creator: %w", err)
	}
	return &creatorGuard{file: f}, nil
}

func (g *creatorGuard) close() error {
	if g.file == nil {
		return nil
	}
	err := g.file.Close()
	g.file = nil
	if err != nil {
		return fmt.Errorf("close creator lease: %w", err)
	}
	return nil
}

type creatorRecord struct {
	ID         string    `json:"id"`
	ItemsHash  string    `json:"items_hash"`
	At         time.Time `json:"at"`
	Dispatched bool      `json:"dispatched"`
}

func (r *Runner) finishCreator(s Submission, g *creatorGuard, dispatched bool) error {
	at := r.Now().UTC().Truncate(time.Microsecond)
	if at.Before(s.CreatedAt) {
		at = s.CreatedAt.UTC()
	}
	b, err := json.Marshal(creatorRecord{ID: s.ID, ItemsHash: s.ItemsHash, At: at, Dispatched: dispatched})
	if err != nil {
		return errors.Join(fmt.Errorf("encode creator completion: %w", err), g.close())
	}
	err = writeOnce(filepath.Join(r.IntentDir, "submissions", s.ID+".creator.finished"), b)
	return errors.Join(err, g.close())
}

func (r *Runner) readCreatorRecord(s Submission, suffix string) (creatorRecord, bool, error) {
	b, err := os.ReadFile(filepath.Join(r.IntentDir, "submissions", s.ID+suffix))
	if errors.Is(err, os.ErrNotExist) {
		return creatorRecord{}, false, nil
	}
	if err != nil {
		return creatorRecord{}, false, fmt.Errorf("read creator record: %w", err)
	}
	var record creatorRecord
	if err = json.Unmarshal(b, &record); err != nil {
		return creatorRecord{}, false, fmt.Errorf("decode creator record: %w", err)
	}
	if record.ID != s.ID || record.ItemsHash != s.ItemsHash || record.At.Before(s.CreatedAt) {
		return creatorRecord{}, false, fmt.Errorf("invalid creator record")
	}
	return record, true, nil
}

func (r *Runner) creatorQuietSince(s Submission) (time.Time, error) {
	finished, known, err := r.readCreatorRecord(s, ".creator.finished")
	if err != nil {
		return time.Time{}, err
	}
	if known {
		if finished.Dispatched {
			return finished.At, nil
		}
		return s.CreatedAt, nil
	}
	absent, known, err := r.readCreatorRecord(s, ".creator.absent")
	if err != nil {
		return time.Time{}, err
	}
	if known {
		return absent.At, nil
	}
	absent = creatorRecord{ID: s.ID, ItemsHash: s.ItemsHash, At: r.Now().UTC().Truncate(time.Microsecond)}
	b, err := json.Marshal(absent)
	if err != nil {
		return time.Time{}, fmt.Errorf("encode creator absence: %w", err)
	}
	if err = writeOnce(filepath.Join(r.IntentDir, "submissions", s.ID+".creator.absent"), b); err != nil {
		return time.Time{}, err
	}
	return absent.At, nil
}
