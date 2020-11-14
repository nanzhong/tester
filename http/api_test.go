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

const testKey = "key"

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
	r.SetBasicAuth("", testKey)
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
			now := time.Now().Round(time.Second)
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
			now := time.Now().Round(time.Second)
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
			mockDB.EXPECT().AddTest(gomock.Any(), test).Return(nil)

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
			now := time.Now()
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
