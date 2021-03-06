// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/nanzhong/tester/db (interfaces: DB)

// Package db is a generated GoMock package.
package db

import (
	context "context"
	gomock "github.com/golang/mock/gomock"
	uuid "github.com/google/uuid"
	tester "github.com/nanzhong/tester"
	reflect "reflect"
	time "time"
)

// MockDB is a mock of DB interface
type MockDB struct {
	ctrl     *gomock.Controller
	recorder *MockDBMockRecorder
}

// MockDBMockRecorder is the mock recorder for MockDB
type MockDBMockRecorder struct {
	mock *MockDB
}

// NewMockDB creates a new mock instance
func NewMockDB(ctrl *gomock.Controller) *MockDB {
	mock := &MockDB{ctrl: ctrl}
	mock.recorder = &MockDBMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockDB) EXPECT() *MockDBMockRecorder {
	return m.recorder
}

// AddTest mocks base method
func (m *MockDB) AddTest(arg0 context.Context, arg1 *tester.Test) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddTest", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddTest indicates an expected call of AddTest
func (mr *MockDBMockRecorder) AddTest(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddTest", reflect.TypeOf((*MockDB)(nil).AddTest), arg0, arg1)
}

// CompleteRun mocks base method
func (m *MockDB) CompleteRun(arg0 context.Context, arg1 uuid.UUID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CompleteRun", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// CompleteRun indicates an expected call of CompleteRun
func (mr *MockDBMockRecorder) CompleteRun(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CompleteRun", reflect.TypeOf((*MockDB)(nil).CompleteRun), arg0, arg1)
}

// DeleteRun mocks base method
func (m *MockDB) DeleteRun(arg0 context.Context, arg1 uuid.UUID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteRun", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteRun indicates an expected call of DeleteRun
func (mr *MockDBMockRecorder) DeleteRun(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteRun", reflect.TypeOf((*MockDB)(nil).DeleteRun), arg0, arg1)
}

// EnqueueRun mocks base method
func (m *MockDB) EnqueueRun(arg0 context.Context, arg1 *tester.Run) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnqueueRun", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// EnqueueRun indicates an expected call of EnqueueRun
func (mr *MockDBMockRecorder) EnqueueRun(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnqueueRun", reflect.TypeOf((*MockDB)(nil).EnqueueRun), arg0, arg1)
}

// FailRun mocks base method
func (m *MockDB) FailRun(arg0 context.Context, arg1 uuid.UUID, arg2 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FailRun", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// FailRun indicates an expected call of FailRun
func (mr *MockDBMockRecorder) FailRun(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FailRun", reflect.TypeOf((*MockDB)(nil).FailRun), arg0, arg1, arg2)
}

// GetRun mocks base method
func (m *MockDB) GetRun(arg0 context.Context, arg1 uuid.UUID) (*tester.Run, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRun", arg0, arg1)
	ret0, _ := ret[0].(*tester.Run)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRun indicates an expected call of GetRun
func (mr *MockDBMockRecorder) GetRun(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRun", reflect.TypeOf((*MockDB)(nil).GetRun), arg0, arg1)
}

// GetTest mocks base method
func (m *MockDB) GetTest(arg0 context.Context, arg1 uuid.UUID) (*tester.Test, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTest", arg0, arg1)
	ret0, _ := ret[0].(*tester.Test)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTest indicates an expected call of GetTest
func (mr *MockDBMockRecorder) GetTest(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTest", reflect.TypeOf((*MockDB)(nil).GetTest), arg0, arg1)
}

// Init mocks base method
func (m *MockDB) Init(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Init", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Init indicates an expected call of Init
func (mr *MockDBMockRecorder) Init(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Init", reflect.TypeOf((*MockDB)(nil).Init), arg0)
}

// ListFinishedRuns mocks base method
func (m *MockDB) ListFinishedRuns(arg0 context.Context, arg1 int) ([]*tester.Run, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListFinishedRuns", arg0, arg1)
	ret0, _ := ret[0].([]*tester.Run)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListFinishedRuns indicates an expected call of ListFinishedRuns
func (mr *MockDBMockRecorder) ListFinishedRuns(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListFinishedRuns", reflect.TypeOf((*MockDB)(nil).ListFinishedRuns), arg0, arg1)
}

// ListPendingRuns mocks base method
func (m *MockDB) ListPendingRuns(arg0 context.Context) ([]*tester.Run, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListPendingRuns", arg0)
	ret0, _ := ret[0].([]*tester.Run)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListPendingRuns indicates an expected call of ListPendingRuns
func (mr *MockDBMockRecorder) ListPendingRuns(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListPendingRuns", reflect.TypeOf((*MockDB)(nil).ListPendingRuns), arg0)
}

// ListRunSummariesInRange mocks base method
func (m *MockDB) ListRunSummariesInRange(arg0 context.Context, arg1, arg2 time.Time, arg3 time.Duration) ([]*tester.RunSummary, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListRunSummariesInRange", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].([]*tester.RunSummary)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListRunSummariesInRange indicates an expected call of ListRunSummariesInRange
func (mr *MockDBMockRecorder) ListRunSummariesInRange(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListRunSummariesInRange", reflect.TypeOf((*MockDB)(nil).ListRunSummariesInRange), arg0, arg1, arg2, arg3)
}

// ListRunsForPackage mocks base method
func (m *MockDB) ListRunsForPackage(arg0 context.Context, arg1 string, arg2 int) ([]*tester.Run, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListRunsForPackage", arg0, arg1, arg2)
	ret0, _ := ret[0].([]*tester.Run)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListRunsForPackage indicates an expected call of ListRunsForPackage
func (mr *MockDBMockRecorder) ListRunsForPackage(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListRunsForPackage", reflect.TypeOf((*MockDB)(nil).ListRunsForPackage), arg0, arg1, arg2)
}

// ListTests mocks base method
func (m *MockDB) ListTests(arg0 context.Context, arg1 int) ([]*tester.Test, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListTests", arg0, arg1)
	ret0, _ := ret[0].([]*tester.Test)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListTests indicates an expected call of ListTests
func (mr *MockDBMockRecorder) ListTests(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListTests", reflect.TypeOf((*MockDB)(nil).ListTests), arg0, arg1)
}

// ListTestsForPackage mocks base method
func (m *MockDB) ListTestsForPackage(arg0 context.Context, arg1 string, arg2 int) ([]*tester.Test, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListTestsForPackage", arg0, arg1, arg2)
	ret0, _ := ret[0].([]*tester.Test)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListTestsForPackage indicates an expected call of ListTestsForPackage
func (mr *MockDBMockRecorder) ListTestsForPackage(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListTestsForPackage", reflect.TypeOf((*MockDB)(nil).ListTestsForPackage), arg0, arg1, arg2)
}

// ListTestsForPackageInRange mocks base method
func (m *MockDB) ListTestsForPackageInRange(arg0 context.Context, arg1 string, arg2, arg3 time.Time) ([]*tester.Test, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListTestsForPackageInRange", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].([]*tester.Test)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListTestsForPackageInRange indicates an expected call of ListTestsForPackageInRange
func (mr *MockDBMockRecorder) ListTestsForPackageInRange(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListTestsForPackageInRange", reflect.TypeOf((*MockDB)(nil).ListTestsForPackageInRange), arg0, arg1, arg2, arg3)
}

// ResetRun mocks base method
func (m *MockDB) ResetRun(arg0 context.Context, arg1 uuid.UUID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResetRun", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// ResetRun indicates an expected call of ResetRun
func (mr *MockDBMockRecorder) ResetRun(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResetRun", reflect.TypeOf((*MockDB)(nil).ResetRun), arg0, arg1)
}

// StartRun mocks base method
func (m *MockDB) StartRun(arg0 context.Context, arg1 uuid.UUID, arg2 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "StartRun", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// StartRun indicates an expected call of StartRun
func (mr *MockDBMockRecorder) StartRun(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StartRun", reflect.TypeOf((*MockDB)(nil).StartRun), arg0, arg1, arg2)
}
