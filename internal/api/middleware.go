package api

import (
	"log"
	"net/http"
	"time"
)

// statusWriter captures the final HTTP status code and number of bytes written.
// This helps distinguish "handler returned 200" from "client received a response".
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Record implicit 200 responses when handlers write without calling WriteHeader.
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}

	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// loggingMiddleware logs end-to-end request duration and response size for basic observability.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		sw := &statusWriter{
			ResponseWriter: w,
			status:         0,
		}

		next.ServeHTTP(sw, r)

		duration := time.Since(start).Milliseconds()

		log.Printf(
			"method=%s path=%s status=%d bytes=%d dur=%dms",
			r.Method, r.URL.RequestURI(), sw.status, sw.bytes, duration,
		)
	})
}
