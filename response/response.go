package response

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime"
)

// JSON writes a standard JSON response
func JSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to write json response", "error", err)
	}
}

// Error handles error mapping, debug traces, and writing the response
func Error(w http.ResponseWriter, code int, err error) {
	slog.Error(err.Error(), "status", code)

	resultCode := code
	message := err.Error()
	var customErr PublicError
	if errors.As(err, &customErr) {
		message = customErr.UserMessage()
		resultCode = customErr.ErrorStatus()
	}

	resp := map[string]any{"status": resultCode}

	if isDebug() {
		// --- DEBUG MODE ---
		resp["message"] = message
		resp["technical"] = err.Error()

		_, file, line, _ := runtime.Caller(2)

		resp["meta"] = map[string]any{
			"file": file,
			"line": line,
		}
	} else {
		// --- PROD MODE ---
		if resultCode == http.StatusInternalServerError {
			resp["message"] = "Internal Server Error"
		} else {
			resp["message"] = message
		}
	}

	JSON(w, resultCode, resp)
}
