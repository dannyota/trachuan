package worker

import (
	"os"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/workflow"
)

func configureOCR(registry workflow.Registry, db store.DBTX) {
	root := os.Getenv("CORPUS_OCR_STORE")
	if root == "" {
		return
	}
	for _, st := range registry.Stages() {
		if extractor, ok := st.(*extract.Stage); ok {
			extractor.WithOCR(ocrstore.Open(root, db), nil, "")
		}
	}
}
