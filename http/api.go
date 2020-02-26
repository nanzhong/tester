package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/alerting"
	"github.com/nanzhong/tester/db"
	"github.com/nanzhong/tester/slack"
	"github.com/prometheus/client_golang/prometheus"
)

// APIHandler is the http handler for presenting the API.
type APIHandler struct {
	http.Handler

	db           db.DB
	alertManager *alerting.AlertManager
	slackApp     *slack.App
	apiKey       string
}

// NewAPIHandler constructs a new `APIHandler`.
func NewAPIHandler(opts ...Option) *APIHandler {
	defOpts := &options{
		db:           &db.MemDB{},
		alertManager: &alerting.AlertManager{},
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	handler := &APIHandler{
		db:           defOpts.db,
		alertManager: defOpts.alertManager,
		slackApp:     defOpts.slackApp,
		apiKey:       defOpts.apiKey,
	}

	r := mux.NewRouter()

	if handler.apiKey != "" {
		r.Use(handler.ensureAuth)
	}

	r.HandleFunc("/api/tests", LogHandlerFunc(handler.submitTest)).Methods(http.MethodPost)
	r.HandleFunc("/api/tests", LogHandlerFunc(handler.listTests)).Methods(http.MethodGet)
	r.HandleFunc("/api/tests/{test_id}", LogHandlerFunc(handler.getTest)).Methods(http.MethodGet)
	r.HandleFunc("/api/runs/claim", LogHandlerFunc(handler.claimRun)).Methods(http.MethodPost)
	r.HandleFunc("/api/runs/{run_id}/complete", LogHandlerFunc(handler.completeRun)).Methods(http.MethodPost)
	r.HandleFunc("/api/runs/{run_id}/fail", LogHandlerFunc(handler.failRun)).Methods(http.MethodPost)

	if handler.slackApp != nil {
		r.HandleFunc("/api/slack/command", LogHandlerFunc(handler.slackApp.HandleSlackCommand)).Methods(http.MethodPost)
	}

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

	err = h.db.AddTest(r.Context(), &test)
	if err != nil {
		log.Printf("failed to add test: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	runLabels := prometheus.Labels{
		"name":  test.Name,
		"state": test.State.String(),
	}
	RunDurationMetric.With(runLabels).Observe(test.FinishedAt.Sub(test.StartedAt).Seconds())
	RunLastMetric.With(runLabels).Set(float64(test.StartedAt.Unix()))

	if test.State == tester.TBFailed {
		go func() {
			err := h.alertManager.Fire(context.Background(), &alerting.Alert{Test: &test})
			if err != nil {
				log.Printf("failed to fire alert: %w", err)
			}
		}()
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(&test)
}

func (h *APIHandler) listTests(w http.ResponseWriter, r *http.Request) {
	tests, err := h.db.ListTests(r.Context(), 0)
	if err != nil {
		log.Printf("failed to list tests: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tests)
}

func (h *APIHandler) getTest(w http.ResponseWriter, r *http.Request) {
	test, err := h.db.GetTest(r.Context(), mux.Vars(r)["test_id"])
	if err != nil {
		if err == db.ErrNotFound {
			renderAPIError(w, http.StatusNotFound, err)
		} else {
			log.Printf("failed to get tests: %s", err)
			renderAPIError(w, http.StatusInternalServerError, err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&test)
}

func (h *APIHandler) claimRun(w http.ResponseWriter, r *http.Request) {
	var packages []string
	err := json.NewDecoder(r.Body).Decode(&packages)
	if err != nil {
		log.Printf("failed to parse claim request: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	supportedPackages := make(map[string]struct{})
	for _, pkg := range packages {
		supportedPackages[pkg] = struct{}{}
	}

	runs, err := h.db.ListPendingRuns(r.Context())
	if err != nil {
		log.Printf("failed to list runs: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	for _, run := range runs {
		if !run.StartedAt.IsZero() {
			continue
		}

		if _, supported := supportedPackages[run.Package.Name]; supported {
			h.db.StartRun(r.Context(), run.ID)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(run)
			return
		}
	}

	renderAPIError(w, http.StatusNotFound, fmt.Errorf("no runs for packages: %s", strings.Join(packages, ", ")))
}

func (h *APIHandler) completeRun(w http.ResponseWriter, r *http.Request) {
	var testIDs []string
	err := json.NewDecoder(r.Body).Decode(&testIDs)
	if err != nil {
		log.Printf("failed to parse complete run request: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	err = h.db.CompleteRun(r.Context(), mux.Vars(r)["run_id"], testIDs)
	if err != nil {
		log.Printf("failed to complete run: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *APIHandler) failRun(w http.ResponseWriter, r *http.Request) {
	var errorMessage string
	err := json.NewDecoder(r.Body).Decode(&errorMessage)
	if err != nil {
		log.Printf("failed to parse fail run request: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	err = h.db.FailRun(r.Context(), mux.Vars(r)["run_id"], errorMessage)
	if err != nil {
		log.Printf("failed to fail run: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *APIHandler) ensureAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || password != h.apiKey {
			renderAPIError(w, http.StatusUnauthorized, fmt.Errorf("user %s is unauthorized", username))
			return
		}
		next.ServeHTTP(w, r)
	})
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

type claimRunRequest struct {
	Packages []string `json:"packages"`
}
