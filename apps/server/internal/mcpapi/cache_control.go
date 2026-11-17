package mcpapi

import "net/http"

const CacheControl = "no-store, no-transform"

func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", CacheControl)
		next.ServeHTTP(&noStoreWriter{ResponseWriter: w}, r)
		w.Header().Set("Cache-Control", CacheControl)
	})
}

type noStoreWriter struct{ http.ResponseWriter }

func (w *noStoreWriter) WriteHeader(status int) {
	w.Header().Set("Cache-Control", CacheControl)
	w.ResponseWriter.WriteHeader(status)
}

func (w *noStoreWriter) Write(data []byte) (int, error) {
	w.Header().Set("Cache-Control", CacheControl)
	return w.ResponseWriter.Write(data)
}

func (w *noStoreWriter) FlushError() error {
	w.Header().Set("Cache-Control", CacheControl)
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *noStoreWriter) Flush() { _ = w.FlushError() }

func (w *noStoreWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
