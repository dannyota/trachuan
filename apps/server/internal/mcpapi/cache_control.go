package mcpapi

import "net/http"

const CacheControl = "no-store, no-transform"

func NoStore(next http.Handler) http.Handler { return WithCacheControl(CacheControl, next) }

func WithCacheControl(value string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		next.ServeHTTP(&noStoreWriter{ResponseWriter: w, value: value}, r)
		w.Header().Set("Cache-Control", value)
	})
}

type noStoreWriter struct {
	http.ResponseWriter
	value string
}

func (w *noStoreWriter) WriteHeader(status int) {
	w.Header().Set("Cache-Control", w.value)
	w.ResponseWriter.WriteHeader(status)
}

func (w *noStoreWriter) Write(data []byte) (int, error) {
	w.Header().Set("Cache-Control", w.value)
	return w.ResponseWriter.Write(data)
}

func (w *noStoreWriter) FlushError() error {
	w.Header().Set("Cache-Control", w.value)
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *noStoreWriter) Flush() { _ = w.FlushError() }

func (w *noStoreWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
