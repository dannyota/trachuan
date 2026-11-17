package mcpapi

import "net/http"

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(&noStoreWriter{ResponseWriter: w}, r)
		w.Header().Set("Cache-Control", "no-store")
	})
}

type noStoreWriter struct{ http.ResponseWriter }

func (w *noStoreWriter) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}

func (w *noStoreWriter) Write(data []byte) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return w.ResponseWriter.Write(data)
}

func (w *noStoreWriter) FlushError() error {
	w.Header().Set("Cache-Control", "no-store")
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *noStoreWriter) Flush() { _ = w.FlushError() }

func (w *noStoreWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
