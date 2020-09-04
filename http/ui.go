package http

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/db"
	"golang.org/x/sync/errgroup"
)

// UIHandler is the http handler for presenting the web UI.
type UIHandler struct {
	http.Handler

	db       db.DB
	packages []tester.Package

	mu                 sync.Mutex
	hourSummaries      []*tester.RunSummary
	daySummaries       []*tester.RunSummary
	monthSummaries     []*tester.RunSummary
	summariesRefreshAt time.Time
}

// NewUIHandler constructs a new `UIHandler`.
func NewUIHandler(db db.DB, packages []tester.Package) *UIHandler {
	handler := &UIHandler{
		db:       db,
		packages: packages,
	}

	r := mux.NewRouter()
	r.HandleFunc("/", LogHandlerFunc(handler.dashboard)).Methods(http.MethodGet)
	r.HandleFunc("/packages", LogHandlerFunc(handler.listPackages)).Methods(http.MethodGet)
	r.HandleFunc("/packages/{package}", LogHandlerFunc(handler.getPackage)).Methods(http.MethodGet)
	r.HandleFunc("/tests/{test_id}", LogHandlerFunc(handler.getTest)).Methods(http.MethodGet)
	r.HandleFunc("/runs", LogHandlerFunc(handler.listRuns)).Methods(http.MethodGet)
	r.HandleFunc("/runs/{run_id}", LogHandlerFunc(handler.getRun)).Methods(http.MethodGet)
	r.HandleFunc("/run_summary", LogHandlerFunc(handler.getRunSummary)).Methods(http.MethodGet)
	handler.Handler = r

	return handler
}

func (h *UIHandler) RefreshSummaries(ctx context.Context) error {
	now := time.Now().Truncate(5 * time.Minute)

	lastHour := now.Add(-time.Hour)
	lastDay := now.Add(-24 * time.Hour)

	var (
		hourSummaries  []*tester.RunSummary
		daySummaries   []*tester.RunSummary
		monthSummaries []*tester.RunSummary
	)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		hourSummaries, err = h.db.ListRunSummariesForRange(ctx, lastHour, now, 5*time.Minute)
		return err
	})
	eg.Go(func() error {
		var err error
		daySummaries, err = h.db.ListRunSummariesForRange(ctx, lastDay, lastHour.Add(-time.Hour), time.Hour)
		return err
	})
	eg.Go(func() error {
		var err error
		monthSummaries, err = h.db.ListRunSummariesForRange(ctx, now.Add(-30*24*time.Hour), now, 12*time.Hour)
		return err
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.hourSummaries = hourSummaries
	h.daySummaries = daySummaries
	h.monthSummaries = monthSummaries
	h.summariesRefreshAt = now

	return nil
}

func (h *UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Handler.ServeHTTP(w, r)
}

func (h *UIHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	// Check to see if we need to refresh stale summary data.
	h.mu.Lock()
	diff := time.Now().Sub(h.summariesRefreshAt)
	h.mu.Unlock()
	if diff > 5*time.Minute {
		if err := h.RefreshSummaries(r.Context()); err != nil {
			h.RenderError(w, r, err, http.StatusInternalServerError)
			return
		}
	}

	h.mu.Lock()
	hourSummaries := h.hourSummaries
	daySummaries := h.daySummaries
	monthSummaries := h.monthSummaries
	h.mu.Unlock()

	uniquePackages := make(map[string]struct{})
	for _, s := range daySummaries {
		for pkg := range s.PackageSummary {
			if _, ok := uniquePackages[pkg]; ok {
				continue
			}
			uniquePackages[pkg] = struct{}{}
		}
	}
	var allPackages []string
	for pkg := range uniquePackages {
		allPackages = append(allPackages, pkg)
	}
	sort.Slice(allPackages, func(i, j int) bool {
		return allPackages[i] < allPackages[j]
	})

	value := &struct {
		Packages       []string
		HourSummaries  []*tester.RunSummary
		DaySummaries   []*tester.RunSummary
		MonthSummaries []*tester.RunSummary
	}{
		Packages:       allPackages,
		HourSummaries:  hourSummaries,
		DaySummaries:   daySummaries,
		MonthSummaries: monthSummaries,
	}

	h.Render(w, r, "dashboard", value)
}

func (h *UIHandler) listPackages(w http.ResponseWriter, r *http.Request) {
	runsByPackage := make(map[string][]*tester.Run)
	for _, p := range h.packages {
		runs, err := h.db.ListRunsForPackage(r.Context(), p.Name, 5)
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

func (h *UIHandler) getRunSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	begin, err := strconv.Atoi(r.URL.Query().Get("begin"))
	if err != nil {
		h.RenderError(w, r, err, http.StatusBadRequest)
		return
	}
	beginTime := time.Unix(int64(begin), 0)

	window, err := strconv.ParseFloat(r.URL.Query().Get("window"), 64)
	if err != nil {
		h.RenderError(w, r, err, http.StatusBadRequest)
		return
	}
	windowDuration := time.Duration(window) * time.Second

	summaries, err := h.db.ListRunSummariesForRange(ctx, beginTime, beginTime.Add(windowDuration), windowDuration)
	if err != nil {
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	value := &struct {
		Package    string
		RunSummary *tester.RunSummary
	}{
		Package:    r.URL.Query().Get("package"),
		RunSummary: summaries[0],
	}
	h.Render(w, r, "run_summary", value)
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
