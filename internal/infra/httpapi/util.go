package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func renderStatus(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		io.WriteString(w, fmt.Sprintf("%d %s", code, http.StatusText(code))) //nolint: errcheck
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint: errcheck
}
