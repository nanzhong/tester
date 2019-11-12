package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/db"
	"github.com/prometheus/client_golang/prometheus"
)

// APIHandler is the http handler for presenting the API.
type APIHandler struct {
	http.Handler

	db *db.MemDB
}

// NewAPIHandler constructs a new `APIHandler`.
func NewAPIHandler(opts ...Option) *APIHandler {
	defOpts := &options{
		db: &db.MemDB{},
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	handler := &APIHandler{
		db: defOpts.db,
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/tests", LogHandlerFunc(handler.submitTest)).Methods(http.MethodPost)
	r.HandleFunc("/api/tests", LogHandlerFunc(handler.listTests)).Methods(http.MethodGet)
	r.HandleFunc("/api/tests/{test_id}", LogHandlerFunc(handler.getTest)).Methods(http.MethodGet)
	handler.Handler = r

	return handler
}

func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Handler.ServeHTTP(w, r)
}

func (h *APIHandler) submitTest(w http.ResponseWriter, r *http.Request) {
	var test tester.Test
	err := json.NewDecoder(r.Body).Decode(&test)
	if err != nil {
		renderAPIError(w, 400, fmt.Errorf("decoding json: %w", err))
		return
	}

	h.db.AddTest(&test)

	runLabels := prometheus.Labels{
		"name":  test.Name,
		"state": test.State.String(),
	}
	RunDurationMetric.With(runLabels).Observe(test.FinishTime.Sub(test.StartTime).Seconds())
	RunLastMetric.With(runLabels).Set(float64(test.StartTime.Unix()))

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(&test)
}

func (h *APIHandler) listTests(w http.ResponseWriter, r *http.Request) {
	tests := h.db.ListTests()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tests)
}

func (h *APIHandler) getTest(w http.ResponseWriter, r *http.Request) {
	test, err := h.db.GetTest(mux.Vars(r)["test_id"])
	if err != nil {
		if err == db.ErrNotFound {
			renderAPIError(w, http.StatusNotFound, err)
		} else {
			renderAPIError(w, http.StatusInternalServerError, err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&test)
}

func renderAPIError(w http.ResponseWriter, status int, err error) {
	aerr := apiError{
		Status: status,
		Error:  err.Error(),
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(&aerr)
}

type apiError struct {
	Status int    `json:"status"`
	Error  string `json:"error"`
}
