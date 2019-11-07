package http

import (
	"bytes"
	"fmt"
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

	db *db.MemDB
}

type options struct {
	db *db.MemDB
}

// Option is used to inject dependencies into a Server on creation.
type Option func(*options)

// WithDB allows configuring a DB.
func WithDB(db *db.MemDB) Option {
	return func(opts *options) {
		opts.db = db
	}
}

// New constructs a new `Server`.
func New(opts ...Option) *UIHandler {
	defOpts := &options{
		db: &db.MemDB{},
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	server := &UIHandler{
		db:            defOpts.db,
		templateFiles: packr.New("templates", "./templates"),
	}

	r := mux.NewRouter()
	r.HandleFunc("/", LogHandlerFunc(server.listTests)).Methods(http.MethodGet)
	r.HandleFunc("/tests", LogHandlerFunc(server.listTests)).Methods(http.MethodGet)
	r.HandleFunc("/tests/{test_id}", LogHandlerFunc(server.getTest)).Methods(http.MethodGet)
	server.Handler = r

	return server
}

func (s *UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Handler.ServeHTTP(w, r)
}

func (s *UIHandler) listTests(w http.ResponseWriter, r *http.Request) {
	var template string
	switch view := r.URL.Query().Get("view"); view {
	case "recent", "name":
		template = fmt.Sprintf("tests_%s", view)
	default:
		template = "tests"
	}

	tests := s.db.ListTests()
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

	s.render(w, r, template, value)
}

func (s *UIHandler) getTest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	test, err := s.db.GetTest(vars["test_id"])
	if err != nil {
		if err == db.ErrNotFound {
			s.renderError(w, r, err, http.StatusNotFound)
		} else {
			s.renderError(w, r, err, http.StatusInternalServerError)
		}
		return
	}

	value := &struct {
		Test *tester.Test
	}{
		Test: test,
	}

	s.render(w, r, "test_details", value)
}

func (s *UIHandler) render(w http.ResponseWriter, r *http.Request, name string, value interface{}) {
	var b bytes.Buffer
	if err := s.ExecuteTemplate(name, &b, value); err != nil {
		s.renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	b.WriteTo(w)
}

func (s *UIHandler) renderError(w http.ResponseWriter, r *http.Request, err error, status int) {
	value := struct {
		Status int
		Error  error
	}{
		Status: status,
		Error:  err,
	}

	var b bytes.Buffer
	if err := s.ExecuteTemplate("error", &b, value); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%+v", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	b.WriteTo(w)
}
