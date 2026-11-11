package mcpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"unicode/utf8"

	"danny.vn/trachuan/apps/server/internal/credential"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := credential.FromContext(r.Context())
		if !ok || principal.AccountID <= 0 || principal.Scope != "mcp:read" {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Request body too large.", http.StatusRequestEntityTooLarge)
			return
		}
		trimmed := bytes.TrimSpace(body)
		if len(trimmed) > 0 && trimmed[0] == '[' {
			protocolError(w, http.StatusBadRequest, -32600, nil)
			return
		}
		if !utf8.Valid(body) || !json.Valid(body) {
			protocolError(w, http.StatusBadRequest, -32700, nil)
			return
		}
		// Use the SDK decoder so the guard and transport agree on field casing
		// and duplicate fields. The standard decoder matches struct names loosely.
		message, err := jsonrpc.DecodeMessage(body)
		request, isRequest := message.(*jsonrpc.Request)
		if err != nil || !isRequest {
			protocolError(w, http.StatusBadRequest, jsonrpc.CodeInvalidRequest, nil)
			return
		}
		// Listening subscriptions force SSE even with JSONResponse in this SDK.
		if request.Method == "subscriptions/listen" {
			protocolError(w, http.StatusBadRequest, jsonrpc.CodeMethodNotFound, request.ID.Raw())
			return
		}

		r = r.Clone(r.Context())
		r.Header.Del("Mcp-Session-Id")
		r.Body = io.NopCloser(bytes.NewReader(body))
		buffered := &jsonResponse{header: make(http.Header)}
		next.ServeHTTP(buffered, r)
		for key, values := range buffered.header {
			if key != "Mcp-Session-Id" {
				w.Header()[key] = values
			}
		}
		status := buffered.status
		if status == 0 {
			status = http.StatusOK
		}
		data := buffered.body.Bytes()
		// Protocol diagnostics from the SDK can include arbitrary client values.
		// Keep codes and IDs, but never expose diagnostic data or interpolated text.
		var response map[string]json.RawMessage
		if json.Unmarshal(data, &response) == nil && response["error"] != nil {
			var rpcError struct {
				Code int `json:"code"`
			}
			_ = json.Unmarshal(response["error"], &rpcError)
			response["error"], _ = json.Marshal(map[string]any{"code": rpcError.Code, "message": "Protocol error."})
			data, _ = json.Marshal(response)
		} else if status >= 400 {
			data = []byte(http.StatusText(status) + "\n")
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(status)
		_, _ = w.Write(data)
	})
}

func protocolError(w http.ResponseWriter, status, code int, id any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": "Protocol error."}})
}

type jsonResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *jsonResponse) Header() http.Header { return w.header }
func (w *jsonResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *jsonResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}
