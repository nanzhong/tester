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
	packages []*tester.Package

	mu                 sync.Mutex
	hourSummaries      []*tester.RunSummary
	daySummaries       []*tester.RunSummary
	monthSummaries     []*tester.RunSummary
	summariesRefreshAt time.Time
}

// NewUIHandler constructs a new `UIHandler`.
func NewUIHandler(db db.DB, packages []*tester.Package) *UIHandler {
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

func (h *UIHandler) LoadSummaries(ctx context.Context) (packages []string, month, day, hour []*tester.RunSummary, err error) {
	// Check to see if we need to refresh stale summary data.
	h.mu.Lock()
	diff := time.Now().Sub(h.summariesRefreshAt)
	h.mu.Unlock()
	if diff < 5*time.Minute {
		return uniquePackages(h.monthSummaries), h.monthSummaries, h.daySummaries, h.hourSummaries, nil
	}

	now := time.Now().Truncate(5 * time.Minute)

	lastHour := now.Add(-time.Hour)
	lastDay := now.Add(-24 * time.Hour)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		hour, err = h.db.ListRunSummariesInRange(ctx, lastHour, now, 5*time.Minute)
		return err
	})
	eg.Go(func() error {
		var err error
		day, err = h.db.ListRunSummariesInRange(ctx, lastDay, lastHour, time.Hour)
		return err
	})
	eg.Go(func() error {
		var err error
		month, err = h.db.ListRunSummariesInRange(ctx, now.Add(-30*24*time.Hour), lastDay, 12*time.Hour)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, nil, nil, nil, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.hourSummaries = hour
	h.daySummaries = day
	h.monthSummaries = month
	h.summariesRefreshAt = now

	return uniquePackages(month), month, day, hour, nil
}

func (h *UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Handler.ServeHTTP(w, r)
}

type monthlyRunSummary struct {
	HourSummaries  []*tester.RunSummary
	DaySummaries   []*tester.RunSummary
	MonthSummaries []*tester.RunSummary

	Height     int
	HeightDiff int
}

type monthlyPackageRunSummary struct {
	Name           string
	HourSummaries  []*tester.RunSummary
	DaySummaries   []*tester.RunSummary
	MonthSummaries []*tester.RunSummary

	Height     int
	HeightDiff int
}

type dailyPackageRunSummary struct {
	Name          string
	HourSummaries []*tester.RunSummary
	DaySummaries  []*tester.RunSummary

	Height     int
	HeightDiff int
}

func (h *UIHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	_, monthSummaries, daySummaries, hourSummaries, err := h.LoadSummaries(r.Context())
	if err != nil {
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	dailyPackageRunSummaries := make(map[string]*dailyPackageRunSummary)

	for _, pkg := range h.packages {
		dailyPackageRunSummaries[pkg.Name] = &dailyPackageRunSummary{
			Name:          pkg.Name,
			HourSummaries: hourSummaries,
			DaySummaries:  daySummaries,

			Height:     60,
			HeightDiff: 10,
		}
	}

	value := &struct {
		Packages                 []*tester.Package
		OverallMonthlyRunSummary *monthlyRunSummary
		DailyPackageRunSummaries map[string]*dailyPackageRunSummary
	}{
		Packages: h.packages,
		OverallMonthlyRunSummary: &monthlyRunSummary{
			HourSummaries:  hourSummaries,
			DaySummaries:   daySummaries,
			MonthSummaries: monthSummaries,

			Height:     100,
			HeightDiff: 20,
		},
		DailyPackageRunSummaries: dailyPackageRunSummaries,
	}

	h.Render(w, r, "dashboard", value)
}

func (h *UIHandler) listPackages(w http.ResponseWriter, r *http.Request) {
	packages, monthSummaries, daySummaries, hourSummaries, err := h.LoadSummaries(r.Context())
	if err != nil {
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	monthlyPackageRunSummaries := make([]*monthlyPackageRunSummary, len(packages))

	for i, pkg := range packages {
		monthlyPackageRunSummaries[i] = &monthlyPackageRunSummary{
			Name:           pkg,
			HourSummaries:  hourSummaries,
			DaySummaries:   daySummaries,
			MonthSummaries: monthSummaries,

			Height:     60,
			HeightDiff: 10,
		}
	}

	h.Render(w, r, "packages", monthlyPackageRunSummaries)
}

func (h *UIHandler) getPackage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pkg := vars["package"]

	latestRuns, err := h.db.ListRunsForPackage(r.Context(), pkg, 5)
	if err != nil {
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	lastWeek := now.Add(-7 * 24 * time.Hour).UTC()

	monthlyTests, err := h.db.ListTestsForPackageInRange(r.Context(), pkg, lastWeek, now)
	if err != nil {
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}
	monthlyTestsByName := make(map[string][]*tester.Test)
	for _, test := range monthlyTests {
		monthlyTestsByName[test.Result.Name] = append(monthlyTestsByName[test.Result.Name], test)
	}

	packages, monthSummaries, daySummaries, hourSummaries, err := h.LoadSummaries(r.Context())
	if err != nil {
		h.RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	monthlyRunSummary := &monthlyPackageRunSummary{
		Name: pkg,

		Height:     100,
		HeightDiff: 20,
	}

	for _, name := range packages {
		if name == pkg {
			monthlyRunSummary.MonthSummaries = monthSummaries
			monthlyRunSummary.DaySummaries = daySummaries
			monthlyRunSummary.HourSummaries = hourSummaries
			break
		}
	}

	value := &struct {
		Name                     string
		MonthlyPackageRunSummary *monthlyPackageRunSummary
		LatestRuns               []*tester.Run
		TestsByName              map[string][]*tester.Test
		Now                      time.Time
		LastWeek                 time.Time
	}{
		Name:                     pkg,
		MonthlyPackageRunSummary: monthlyRunSummary,
		LatestRuns:               latestRuns,
		TestsByName:              monthlyTestsByName,
		Now:                      now,
		LastWeek:                 lastWeek,
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

	summaries, err := h.db.ListRunSummariesInRange(ctx, beginTime, beginTime.Add(windowDuration), windowDuration)
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

func uniquePackages(summaries []*tester.RunSummary) []string {
	unique := make(map[string]struct{})
	for _, s := range summaries {
		for pkg := range s.PackageSummary {
			if _, ok := unique[pkg]; ok {
				continue
			}
			unique[pkg] = struct{}{}
		}
	}

	var packages []string
	for pkg := range unique {
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i] < packages[j]
	})

	return packages
}
