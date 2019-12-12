package runner

import (
	"strings"
	"time"
)

type testEvent struct {
	Time   time.Time  `json:"time"`
	Action string     `json:"Action"`
	Test   string     `json:"Test"`
	Output *textBytes `json:"Output"`
}

func (e *testEvent) TopLevel() bool {
	return !strings.Contains(e.Test, "/")
}

func (e *testEvent) ParentTest() string {
	parts := strings.Split(e.Test, "/")
	return strings.Join(parts[:len(parts)-1], "/")
}

func (e *testEvent) ParentTests() []string {
	if e.TopLevel() {
		return nil
	}

	var (
		parents []string
		name    string
	)
	parts := strings.Split(e.Test, "/")
	for _, part := range parts {
		name = name + part
		parents = append(parents, name)
		name = name + "/"
	}
	return parents
}

// https://github.com/golang/go/blob/master/src/cmd/internal/test2json/test2json.go#L44
type textBytes []byte

func (b *textBytes) UnmarshalText(text []byte) error {
	*b = text
	return nil
}

func (b textBytes) Bytes() []byte {
	return []byte(b)
}
