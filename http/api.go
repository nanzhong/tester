package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
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
	packages     map[string]*tester.Package
	alertManager *alerting.AlertManager
	slackApp     *slack.App
	apiKey       string
}

// NewAPIHandler constructs a new `APIHandler`.
func NewAPIHandler(db db.DB, packages []*tester.Package, opts ...Option) *APIHandler {
	defOpts := &options{
		alertManager: &alerting.AlertManager{},
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	handler := &APIHandler{
		db:           db,
		packages:     make(map[string]*tester.Package),
		alertManager: defOpts.alertManager,
		slackApp:     defOpts.slackApp,
		apiKey:       defOpts.apiKey,
	}

	for _, pkg := range packages {
		handler.packages[pkg.Name] = pkg
	}

	r := mux.NewRouter()

	if handler.slackApp != nil {
		r.HandleFunc("/api/slack/command", LogHandlerFunc(handler.slackApp.HandleSlackCommand)).Methods(http.MethodPost)
	}

	ar := r.PathPrefix("/api").Subrouter()
	if handler.apiKey != "" {
		ar.Use(handler.ensureAuth)
	}
	ar.HandleFunc("/tests", LogHandlerFunc(handler.submitTest)).Methods(http.MethodPost)
	ar.HandleFunc("/tests", LogHandlerFunc(handler.listTests)).Methods(http.MethodGet)
	ar.HandleFunc("/tests/{test_id}", LogHandlerFunc(handler.getTest)).Methods(http.MethodGet)
	ar.HandleFunc("/runs/claim", LogHandlerFunc(handler.claimRun)).Methods(http.MethodPost)
	ar.HandleFunc("/runs/{run_id}/complete", LogHandlerFunc(handler.completeRun)).Methods(http.MethodPost)
	ar.HandleFunc("/runs/{run_id}/fail", LogHandlerFunc(handler.failRun)).Methods(http.MethodPost)
	ar.HandleFunc("/packages/{package_name}", LogHandlerFunc(handler.getPackage)).Methods(http.MethodGet)
	ar.HandleFunc("/packages/{package_name}/download", LogHandlerFunc(handler.downloadPackage)).Methods(http.MethodGet)

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
		renderAPIError(w, http.StatusBadRequest, fmt.Errorf("decoding json: %w", err))
		return
	}

	run, err := h.db.GetRun(r.Context(), test.RunID)
	if err != nil {
		renderAPIError(w, http.StatusInternalServerError, fmt.Errorf("getting run: %w", err))
		return
	}
	if !run.FinishedAt.IsZero() {
		renderAPIError(w, http.StatusBadRequest, errors.New("cannot submit test for finished run"))
		return
	}

	err = h.db.AddTest(r.Context(), &test)
	if err != nil {
		log.Printf("failed to add test: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	runLabels := prometheus.Labels{
		"name":  test.Result.Name,
		"state": string(test.Result.State),
	}
	RunDurationMetric.With(runLabels).Observe(test.Result.FinishedAt.Sub(test.Result.StartedAt).Seconds())
	RunLastMetric.With(runLabels).Set(float64(test.Result.StartedAt.Unix()))

	if test.Result.State == tester.TBStateFailed {
		go func() {
			err := h.alertManager.Fire(context.Background(), &alerting.Alert{Run: run, Test: &test})
			if err != nil {
				log.Printf("failed to fire alert: %s", err)
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
	testID, err := uuid.Parse(mux.Vars(r)["test_id"])
	if err != nil {
		renderAPIError(w, http.StatusNotFound, err)
		return
	}

	test, err := h.db.GetTest(r.Context(), testID)
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

type ClaimRunRequest struct {
	PackageWhitelist []string `json:"package_whitelist"`
	PackageBlacklist []string `json:"package_blacklist"`
}

func (h *APIHandler) claimRun(w http.ResponseWriter, r *http.Request) {
	var claimRunRequest ClaimRunRequest
	err := json.NewDecoder(r.Body).Decode(&claimRunRequest)
	if err != nil {
		log.Printf("failed to parse claim run request: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	var packages []string
	if len(claimRunRequest.PackageWhitelist) == 0 {
		for _, pkg := range h.packages {
			packages = append(packages, pkg.Name)
		}
	} else {
		packages = claimRunRequest.PackageWhitelist
	}
	supportedPackages := make(map[string]struct{})
	for _, pkg := range packages {
		supportedPackages[pkg] = struct{}{}
	}

	unsupportedPackages := make(map[string]struct{})
	for _, pkg := range claimRunRequest.PackageBlacklist {
		unsupportedPackages[pkg] = struct{}{}
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

		if _, unsupported := unsupportedPackages[run.Package]; unsupported {
			continue
		}

		if _, supported := supportedPackages[run.Package]; supported {
			h.db.StartRun(r.Context(), run.ID, r.Header.Get("User-Agent"))
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(run)
			return
		}
	}

	renderAPIError(w, http.StatusNotFound, fmt.Errorf("no runs for packages: %s", strings.Join(packages, ", ")))
}

func (h *APIHandler) completeRun(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(mux.Vars(r)["run_id"])
	if err != nil {
		renderAPIError(w, http.StatusNotFound, err)
		return
	}

	run, err := h.db.GetRun(r.Context(), runID)
	if err != nil {
		renderAPIError(w, http.StatusInternalServerError, fmt.Errorf("getting run: %w", err))
		return
	}
	if !run.FinishedAt.IsZero() {
		renderAPIError(w, http.StatusBadRequest, errors.New("cannot complete already finished run"))
		return
	}

	err = h.db.CompleteRun(r.Context(), runID)
	if err != nil {
		log.Printf("failed to complete run: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *APIHandler) failRun(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(mux.Vars(r)["run_id"])
	if err != nil {
		renderAPIError(w, http.StatusNotFound, err)
		return
	}

	run, err := h.db.GetRun(r.Context(), runID)
	if err != nil {
		renderAPIError(w, http.StatusInternalServerError, fmt.Errorf("getting run: %w", err))
		return
	}
	if !run.FinishedAt.IsZero() {
		renderAPIError(w, http.StatusBadRequest, errors.New("cannot fail already finished run"))
		return
	}

	var errorMessage string
	err = json.NewDecoder(r.Body).Decode(&errorMessage)
	if err != nil {
		log.Printf("failed to parse fail run request: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	err = h.db.FailRun(r.Context(), runID, errorMessage)
	if err != nil {
		log.Printf("failed to fail run: %s", err)
		renderAPIError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *APIHandler) getPackage(w http.ResponseWriter, r *http.Request) {
	pkgName := mux.Vars(r)["package_name"]
	pkg, ok := h.packages[pkgName]
	if !ok {
		renderAPIError(w, http.StatusNotFound, fmt.Errorf("package %s not found", pkgName))
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&pkg)
}

func (h *APIHandler) downloadPackage(w http.ResponseWriter, r *http.Request) {
	pkgName := mux.Vars(r)["package_name"]
	pkg, ok := h.packages[pkgName]
	if !ok {
		renderAPIError(w, http.StatusNotFound, fmt.Errorf("package %s not found", pkgName))
		return
	}

	http.ServeFile(w, r, pkg.Path)
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
