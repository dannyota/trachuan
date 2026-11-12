package fetch

import (
	"context"
	"errors"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

// The document-wide PDF ceiling is set in docs/design/budgets.md.
const pdfDownloadLimit = 10 * time.Minute

var errPDFDeadline = errors.New("document PDF deadline exhausted")

type pdfBudget struct {
	deadline time.Time
	now      func() time.Time
}

func newPDFBudget(now func() time.Time) pdfBudget {
	return pdfBudget{deadline: now().Add(pdfDownloadLimit), now: now}
}
func (b pdfBudget) download(ctx context.Context, s *Stage, dir, url string) (chinhphu.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return chinhphu.FileInfo{}, err
	}
	remaining := b.deadline.Sub(b.now())
	if remaining <= 0 {
		return chinhphu.FileInfo{}, errPDFDeadline
	}
	downloadCtx, cancel := context.WithTimeout(ctx, remaining)
	defer cancel()
	info, err := s.downloadChinhphu(downloadCtx, dir, url)
	status := 0
	var failure *chinhphu.Failure
	if errors.As(err, &failure) {
		status = failure.HTTPStatus
	}
	if contextErr := sourcegate.OutcomeContextError(ctx, status, http.StatusOK, err); contextErr != nil {
		return chinhphu.FileInfo{}, contextErr
	}
	if errors.Is(err, context.DeadlineExceeded) && (downloadCtx.Err() != nil || !b.now().Before(b.deadline)) {
		return chinhphu.FileInfo{}, errPDFDeadline
	}
	return info, err
}
