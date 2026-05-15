// Package middleware holds http.Handler wrappers shared across the
// service. Logger is the only thing here today.
package middleware

import (
	"log"
	"net/http"
	"time"
)

// statusRecorder captures the status code an inner handler writes so
// the logger can include it in the access line.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// RequestLogger returns a wrapper that logs `<method> <path> <status>
// <duration>` for every served request. The logger is injected so
// tests can capture output.
func RequestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
		})
	}
}
