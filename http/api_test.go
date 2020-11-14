package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/nanzhong/tester"
	"github.com/nanzhong/tester/db"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

const (
	testKey       = "key"
	testUserAgent = "tester/test"
)

func withAPIHandler(t *testing.T, fn func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB)) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := db.NewMockDB(ctrl)
	api := NewAPIHandler(mockDB, nil, WithAPIKey(testKey))
	ts := httptest.NewServer(api)
	defer ts.Close()

	fn(ts, api, mockDB)
}

func assertAPIAuth(t *testing.T, method, path string, body io.Reader) {
	withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
		t.Run("missing auth", func(t *testing.T) {
			req, err := http.NewRequest(method, fmt.Sprintf("%s%s", ts.URL, path), body)
			require.NoError(t, err)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})
	})
}
func addAuth(r *http.Request) {
	r.SetBasicAuth(testUserAgent, testKey)
	r.Header.Set("User-Agent", testUserAgent)
}

func TestSubmitTest(t *testing.T) {
	t.Run("api auth", func(t *testing.T) {
		test := &tester.Test{}
		reqBody, err := json.Marshal(test)
		require.NoError(t, err)

		assertAPIAuth(t, http.MethodPost, "/api/tests", bytes.NewBuffer(reqBody))
	})

	t.Run("invalid request body", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/tests", ts.URL), bytes.NewBufferString("invalid"))
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("already finished run", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			now := time.Now().UTC().Round(time.Second)
			test := &tester.Test{
				RunID: uuid.New(),
			}
			reqBody, err := json.Marshal(test)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/tests", ts.URL), bytes.NewBuffer(reqBody))
			require.NoError(t, err)

			addAuth(req)

			mockDB.EXPECT().GetRun(gomock.Any(), test.RunID).Return(&tester.Run{FinishedAt: now}, nil)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("happy path", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			now := time.Now().UTC().Round(time.Second)
			test := &tester.Test{
				ID:      uuid.New(),
				Package: "pkg",
				RunID:   uuid.New(),
				Result: &tester.T{
					TB: tester.TB{
						Name:       "TestA",
						StartedAt:  now,
						FinishedAt: now,
						State:      tester.TBStatePassed,
					},
				},
				Logs: []tester.TBLog{{
					Time:   now,
					Name:   "TestA",
					Output: []byte("output"),
				}},
			}
			reqBody, err := json.Marshal(test)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/tests", ts.URL), bytes.NewBuffer(reqBody))
			require.NoError(t, err)

			addAuth(req)

			mockDB.EXPECT().GetRun(gomock.Any(), test.RunID).Return(&tester.Run{}, nil)
			mockDB.EXPECT().AddTest(gomock.Any(), gomock.Eq(test)).Return(nil)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusAccepted, resp.StatusCode)

			var respTest tester.Test
			err = json.NewDecoder(resp.Body).Decode(&respTest)
			require.NoError(t, err)
			assert.DeepEqual(t, test, &respTest)
		})
	})
}

func TestListTests(t *testing.T) {
	t.Run("api auth", func(t *testing.T) {
		assertAPIAuth(t, http.MethodGet, "/api/tests", nil)
	})

	t.Run("happy path", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			now := time.Now().UTC().Round(time.Second)
			tests := []*tester.Test{{
				ID:      uuid.New(),
				Package: "pkg",
				RunID:   uuid.New(),
				Result: &tester.T{
					TB: tester.TB{
						Name:       "TestA",
						StartedAt:  now,
						FinishedAt: now,
						State:      tester.TBStatePassed,
					},
				},
				Logs: []tester.TBLog{{
					Time:   now,
					Name:   "TestA",
					Output: []byte("output"),
				}},
			}}

			mockDB.EXPECT().ListTests(gomock.Any(), 0).Return(tests, nil)

			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/tests", ts.URL), nil)
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var respTests []*tester.Test
			err = json.NewDecoder(resp.Body).Decode(&respTests)
			require.NoError(t, err)
			assert.DeepEqual(t, tests, respTests)
		})
	})
}

func TestGetTest(t *testing.T) {
	t.Run("api auth", func(t *testing.T) {
		assertAPIAuth(t, http.MethodGet, fmt.Sprintf("/api/tests/%s", uuid.New()), nil)
	})

	t.Run("test not found", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			missingID := uuid.New()
			mockDB.EXPECT().GetTest(gomock.Any(), gomock.Eq(missingID)).Return(nil, db.ErrNotFound)

			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/tests/%s", ts.URL, missingID), nil)
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})

	t.Run("happy path", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			now := time.Now().UTC().Round(time.Second)
			test := &tester.Test{
				ID:      uuid.New(),
				Package: "pkg",
				RunID:   uuid.New(),
				Result: &tester.T{
					TB: tester.TB{
						Name:       "TestA",
						StartedAt:  now,
						FinishedAt: now,
						State:      tester.TBStatePassed,
					},
				},
				Logs: []tester.TBLog{{
					Time:   now,
					Name:   "TestA",
					Output: []byte("output"),
				}},
			}

			mockDB.EXPECT().GetTest(gomock.Any(), test.ID).Return(test, nil)

			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/tests/%s", ts.URL, test.ID), nil)
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var respTest tester.Test
			err = json.NewDecoder(resp.Body).Decode(&respTest)
			require.NoError(t, err)
			assert.DeepEqual(t, test, &respTest)
		})
	})
}

func TestClaimRun(t *testing.T) {
	t.Run("api auth", func(t *testing.T) {
		req := ClaimRunRequest{
			PackageWhitelist: []string{},
			PackageBlacklist: []string{},
		}
		reqBody, err := json.Marshal(&req)
		require.NoError(t, err)

		assertAPIAuth(t, http.MethodPost, "/api/runs/claim", bytes.NewBuffer(reqBody))
	})

	t.Run("happy path - no whitelist", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			api.packages = map[string]*tester.Package{"pkg": {
				Name: "pkg",
			}}

			now := time.Now().UTC().Round(time.Second)
			run := &tester.Run{
				ID:         uuid.New(),
				Package:    "pkg",
				EnqueuedAt: now,
			}

			mockDB.EXPECT().ListPendingRuns(gomock.Any()).Return([]*tester.Run{run}, nil)
			mockDB.EXPECT().StartRun(gomock.Any(), run.ID, testUserAgent).Return(nil)

			claimReq := ClaimRunRequest{
				PackageWhitelist: []string{},
				PackageBlacklist: []string{},
			}
			reqBody, err := json.Marshal(&claimReq)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/runs/claim", ts.URL), bytes.NewBuffer(reqBody))
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var respRun tester.Run
			err = json.NewDecoder(resp.Body).Decode(&respRun)
			require.NoError(t, err)
			assert.DeepEqual(t, run, &respRun)
		})
	})

	t.Run("happy path - whitelist", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			api.packages = map[string]*tester.Package{
				"pkg1": {Name: "pkg1"},
				"pkg2": {Name: "pkg2"},
			}

			now := time.Now().UTC().Round(time.Second)
			runs := []*tester.Run{
				{
					ID:         uuid.New(),
					Package:    "pkg1",
					EnqueuedAt: now,
				},
				{
					ID:         uuid.New(),
					Package:    "pkg2",
					EnqueuedAt: now,
				},
			}

			mockDB.EXPECT().ListPendingRuns(gomock.Any()).Return(runs, nil)
			mockDB.EXPECT().StartRun(gomock.Any(), runs[1].ID, testUserAgent).Return(nil)

			claimReq := ClaimRunRequest{
				PackageWhitelist: []string{"pkg2"},
				PackageBlacklist: []string{},
			}
			reqBody, err := json.Marshal(&claimReq)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/runs/claim", ts.URL), bytes.NewBuffer(reqBody))
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var respRun tester.Run
			err = json.NewDecoder(resp.Body).Decode(&respRun)
			require.NoError(t, err)
			assert.DeepEqual(t, runs[1], &respRun)
		})
	})

	t.Run("happy path - blacklist", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			api.packages = map[string]*tester.Package{
				"pkg1": {Name: "pkg1"},
				"pkg2": {Name: "pkg2"},
			}

			now := time.Now().UTC().Round(time.Second)
			runs := []*tester.Run{
				{
					ID:         uuid.New(),
					Package:    "pkg1",
					EnqueuedAt: now,
				},
				{
					ID:         uuid.New(),
					Package:    "pkg2",
					EnqueuedAt: now,
				},
			}

			mockDB.EXPECT().ListPendingRuns(gomock.Any()).Return(runs, nil)
			mockDB.EXPECT().StartRun(gomock.Any(), runs[1].ID, testUserAgent).Return(nil)

			claimReq := ClaimRunRequest{
				PackageWhitelist: []string{"pkg1", "pkg2"},
				PackageBlacklist: []string{"pkg1"},
			}
			reqBody, err := json.Marshal(&claimReq)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/runs/claim", ts.URL), bytes.NewBuffer(reqBody))
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var respRun tester.Run
			err = json.NewDecoder(resp.Body).Decode(&respRun)
			require.NoError(t, err)
			assert.DeepEqual(t, runs[1], &respRun)
		})
	})
}

func TestCompleteRun(t *testing.T) {
	t.Run("api auth", func(t *testing.T) {
		assertAPIAuth(t, http.MethodPost, fmt.Sprintf("/api/runs/%s/complete", uuid.New()), nil)
	})

	t.Run("already finished run", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			now := time.Now().UTC().Round(time.Second)
			run := &tester.Run{
				ID:         uuid.New(),
				FinishedAt: now,
			}
			mockDB.EXPECT().GetRun(gomock.Any(), gomock.Eq(run.ID)).Return(run, nil)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/runs/%s/complete", ts.URL, run.ID), nil)
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("happy path", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			run := &tester.Run{
				ID: uuid.New(),
			}
			mockDB.EXPECT().GetRun(gomock.Any(), gomock.Eq(run.ID)).Return(run, nil)
			mockDB.EXPECT().CompleteRun(gomock.Any(), gomock.Eq(run.ID)).Return(nil)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/runs/%s/complete", ts.URL, run.ID), nil)
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	})
}

func TestFailRun(t *testing.T) {
	t.Run("api auth", func(t *testing.T) {
		errorMsg := "error"
		reqBody, err := json.Marshal(&errorMsg)
		require.NoError(t, err)

		assertAPIAuth(t, http.MethodPost, fmt.Sprintf("/api/runs/%s/fail", uuid.New()), bytes.NewBuffer(reqBody))
	})

	t.Run("already finished run", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			now := time.Now().UTC().Round(time.Second)
			run := &tester.Run{
				ID:         uuid.New(),
				FinishedAt: now,
			}
			mockDB.EXPECT().GetRun(gomock.Any(), gomock.Eq(run.ID)).Return(run, nil)

			errorMsg := "error"
			reqBody, err := json.Marshal(&errorMsg)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/runs/%s/fail", ts.URL, run.ID), bytes.NewBuffer(reqBody))
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("happy path", func(t *testing.T) {
		withAPIHandler(t, func(ts *httptest.Server, api *APIHandler, mockDB *db.MockDB) {
			errorMsg := "error"
			run := &tester.Run{
				ID: uuid.New(),
			}
			mockDB.EXPECT().GetRun(gomock.Any(), gomock.Eq(run.ID)).Return(run, nil)
			mockDB.EXPECT().FailRun(gomock.Any(), gomock.Eq(run.ID), gomock.Eq(errorMsg)).Return(nil)

			reqBody, err := json.Marshal(&errorMsg)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/runs/%s/fail", ts.URL, run.ID), bytes.NewBuffer(reqBody))
			require.NoError(t, err)

			addAuth(req)

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	})
}
