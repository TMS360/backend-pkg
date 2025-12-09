package response

import "sync/atomic"

var debugMode atomic.Bool

// SetDebug configures the response package. Call this from main.go
func SetDebug(debug bool) {
	debugMode.Store(debug)
}

func isDebug() bool {
	return debugMode.Load()
}
