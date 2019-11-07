package http

import (
	"log"
	"net/http"
	"time"
)

// ResponseInspectingWriter is an http.ResponseWriter that captures response info.
type ResponseInspectingWriter struct {
	http.ResponseWriter
	Status int
}

// WriteHeader wraps the method capturing the response status code.
func (w *ResponseInspectingWriter) WriteHeader(s int) {
	w.Status = s
	w.ResponseWriter.WriteHeader(s)
}

// Write wraps the method for writing response bodies..
func (w *ResponseInspectingWriter) Write(p []byte) (int, error) {
	// By default if a http.ResponseWriter's WriteHeader has not been called
	// before Write is called, Write will default to returning a status of 200. We
	// check for this case by seeing if Status == 0 (nil value of int).
	if w.Status == 0 {
		w.Status = http.StatusOK
	}

	return w.ResponseWriter.Write(p)
}

var _ http.ResponseWriter = &ResponseInspectingWriter{}

// LogHandlerFunc logs request/response information.
func LogHandlerFunc(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		riw := &ResponseInspectingWriter{ResponseWriter: w}
		start := time.Now()

		log.Printf("received %s %s\n", r.Method, r.URL.String())

		next.ServeHTTP(riw, r)

		duration := time.Since(start).Seconds()
		log.Printf("handled %s %s [%d] (%fs)\n", r.Method, r.URL.String(), riw.Status, duration)
	})
}
