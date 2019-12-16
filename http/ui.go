package http

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"sort"

	packr "github.com/gobuffalo/packr/v2"
	"github.com/gorilla/mux"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/db"
)

// UIHandler is the http handler for presenting the web UI.
type UIHandler struct {
	http.Handler

	templateFiles *packr.Box

	db db.DB
}

// NewUIHandler constructs a new `UIHandler`.
func NewUIHandler(opts ...Option) *UIHandler {
	defOpts := &options{
		db: &db.MemDB{},
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	handler := &UIHandler{
		db:            defOpts.db,
		templateFiles: packr.New("templates", "./templates"),
	}

	r := mux.NewRouter()
	r.HandleFunc("/", LogHandlerFunc(handler.listTests)).Methods(http.MethodGet)
	r.HandleFunc("/tests", LogHandlerFunc(handler.listTests)).Methods(http.MethodGet)
	r.HandleFunc("/tests/{test_id}", LogHandlerFunc(handler.getTest)).Methods(http.MethodGet)
	r.HandleFunc("/runs", LogHandlerFunc(handler.listRuns)).Methods(http.MethodGet)
	r.HandleFunc("/runs/{run_id}", LogHandlerFunc(handler.getRun)).Methods(http.MethodGet)
	handler.Handler = r

	return handler
}

func (h *UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Handler.ServeHTTP(w, r)
}

func (h *UIHandler) listTests(w http.ResponseWriter, r *http.Request) {
	var template string
	switch view := r.URL.Query().Get("view"); view {
	case "recent", "name":
		template = fmt.Sprintf("tests_%s", view)
	default:
		template = "tests"
	}

	tests, err := h.db.ListTests(r.Context(), 0)
	if err != nil {
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	value := &struct {
		Tests       []*tester.Test
		TestNames   []string
		TestsByName map[string][]*tester.Test
	}{
		Tests:       tests,
		TestsByName: make(map[string][]*tester.Test),
	}

	for _, test := range tests {
		value.TestsByName[test.Name] = append(value.TestsByName[test.Name], test)
	}

	for name := range value.TestsByName {
		value.TestNames = append(value.TestNames, name)
	}
	sort.Strings(value.TestNames)

	h.render(w, r, template, value)
}

func (h *UIHandler) getTest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	test, err := h.db.GetTest(r.Context(), vars["test_id"])
	if err != nil {
		if err == db.ErrNotFound {
			h.renderError(w, r, err, http.StatusNotFound)
		} else {
			h.renderError(w, r, err, http.StatusInternalServerError)
		}
		return
	}

	value := &struct {
		Test *tester.Test
	}{
		Test: test,
	}

	h.render(w, r, "test_details", value)
}

func (h *UIHandler) listRuns(w http.ResponseWriter, r *http.Request) {
	pendingRuns, err := h.db.ListPendingRuns(r.Context())
	if err != nil {
		log.Printf("failed to list runs: %s", err)
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	finishedRuns, err := h.db.ListFinishedRuns(r.Context(), 50)
	if err != nil {
		log.Printf("failed to list runs: %s", err)
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	value := &struct {
		PendingRuns  []*tester.Run
		FinishedRuns []*tester.Run
	}{
		PendingRuns:  pendingRuns,
		FinishedRuns: finishedRuns,
	}

	h.render(w, r, "runs", value)
}

func (h *UIHandler) getRun(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	run, err := h.db.GetRun(r.Context(), vars["run_id"])
	if err != nil {
		if err == db.ErrNotFound {
			h.renderError(w, r, err, http.StatusNotFound)
		} else {
			h.renderError(w, r, err, http.StatusInternalServerError)
		}
		return
	}

	value := &struct {
		Run *tester.Run
	}{
		Run: run,
	}

	h.render(w, r, "run_details", value)
}

func (h *UIHandler) render(w http.ResponseWriter, r *http.Request, name string, value interface{}) {
	var b bytes.Buffer
	if err := h.ExecuteTemplate(name, &b, value); err != nil {
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	b.WriteTo(w)
}

func (h *UIHandler) renderError(w http.ResponseWriter, r *http.Request, err error, status int) {
	value := struct {
		Status int
		Error  error
	}{
		Status: status,
		Error:  err,
	}

	var b bytes.Buffer
	if err := h.ExecuteTemplate("error", &b, value); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%+v", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	b.WriteTo(w)
}
