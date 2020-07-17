package http

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"sort"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/db"
)

// UIHandler is the http handler for presenting the web UI.
type UIHandler struct {
	http.Handler

	db       db.DB
	packages []tester.Package
}

// NewUIHandler constructs a new `UIHandler`.
func NewUIHandler(db db.DB, packages []tester.Package) *UIHandler {
	handler := &UIHandler{
		db:       db,
		packages: packages,
	}

	r := mux.NewRouter()
	r.HandleFunc("/", LogHandlerFunc(handler.listPackages)).Methods(http.MethodGet)
	r.HandleFunc("/packages", LogHandlerFunc(handler.listPackages)).Methods(http.MethodGet)
	r.HandleFunc("/packages/{package}", LogHandlerFunc(handler.getPackage)).Methods(http.MethodGet)
	r.HandleFunc("/tests/{test_id}", LogHandlerFunc(handler.getTest)).Methods(http.MethodGet)
	r.HandleFunc("/runs", LogHandlerFunc(handler.listRuns)).Methods(http.MethodGet)
	r.HandleFunc("/runs/{run_id}", LogHandlerFunc(handler.getRun)).Methods(http.MethodGet)
	handler.Handler = r

	return handler
}

func (h *UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Handler.ServeHTTP(w, r)
}

func (h *UIHandler) listPackages(w http.ResponseWriter, r *http.Request) {
	runsByPackage := make(map[string][]*tester.Run)
	for _, p := range h.packages {
		runs, err := h.db.ListRunsForPackage(r.Context(), p.Name, 15)
		if err != nil {
			h.RenderError(w, r, err, http.StatusInternalServerError)
			return
		}
		runsByPackage[p.Name] = runs
	}

	value := &struct {
		Packages []struct {
			Name string
			Runs []*tester.Run
		}
	}{}
	for pkg, runs := range runsByPackage {
		value.Packages = append(value.Packages, struct {
			Name string
			Runs []*tester.Run
		}{
			Name: pkg,
			Runs: runs,
		})
	}

	sort.Slice(value.Packages, func(i, j int) bool {
		return value.Packages[i].Name < value.Packages[j].Name
	})

	h.Render(w, r, "packages", value)
}

func (h *UIHandler) getPackage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pkg := vars["package"]
	runs, err := h.db.ListRunsForPackage(r.Context(), pkg, 50)
	if err != nil {
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	value := &struct {
		Name string
		Runs []*tester.Run
	}{
		Name: pkg,
		Runs: runs,
	}

	h.Render(w, r, "package_details", value)
}

func (h *UIHandler) getTest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	testID, err := uuid.Parse(vars["test_id"])
	if err != nil {
		h.RenderError(w, r, err, http.StatusNotFound)
		return
	}

	test, err := h.db.GetTest(r.Context(), testID)
	if err != nil {
		if err == db.ErrNotFound {
			h.RenderError(w, r, err, http.StatusNotFound)
		} else {
			h.RenderError(w, r, err, http.StatusInternalServerError)
		}
		return
	}

	value := &struct {
		Test *tester.Test
	}{
		Test: test,
	}

	h.Render(w, r, "test_details", value)
}

func (h *UIHandler) listRuns(w http.ResponseWriter, r *http.Request) {
	pendingRuns, err := h.db.ListPendingRuns(r.Context())
	if err != nil {
		log.Printf("failed to list runs: %s", err)
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	finishedRuns, err := h.db.ListFinishedRuns(r.Context(), 50)
	if err != nil {
		log.Printf("failed to list runs: %s", err)
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	value := &struct {
		PendingRuns  []*tester.Run
		FinishedRuns []*tester.Run
	}{
		PendingRuns:  pendingRuns,
		FinishedRuns: finishedRuns,
	}

	h.Render(w, r, "runs", value)
}

func (h *UIHandler) getRun(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	runID, err := uuid.Parse(vars["run_id"])
	if err != nil {
		h.RenderError(w, r, err, http.StatusNotFound)
		return
	}

	run, err := h.db.GetRun(r.Context(), runID)
	if err != nil {
		if err == db.ErrNotFound {
			h.RenderError(w, r, err, http.StatusNotFound)
		} else {
			h.RenderError(w, r, err, http.StatusInternalServerError)
		}
		return
	}

	value := &struct {
		Run *tester.Run
	}{
		Run: run,
	}

	h.Render(w, r, "run_details", value)
}

func (h *UIHandler) Render(w http.ResponseWriter, r *http.Request, name string, value interface{}) {
	var b bytes.Buffer
	if err := h.ExecuteTemplate(name, &b, value); err != nil {
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	b.WriteTo(w)
}

func (h *UIHandler) RenderError(w http.ResponseWriter, r *http.Request, err error, status int) {
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
