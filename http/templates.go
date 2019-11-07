package http

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/gobuffalo/packd"
	"github.com/nanzhong/tester"
)

var errTemplateNotFound = errors.New("template not found")

// ExecuteTemplate runs the given template with the value
func (s *UIHandler) ExecuteTemplate(name string, w io.Writer, value interface{}) error {
	layoutContent, err := s.templateFiles.Find("layouts/default.html")
	if err != nil {
		return errTemplateNotFound
	}

	layout, err := template.New("layout_default").Funcs(s.templateFuncs()).Parse(string(layoutContent))
	if err != nil {
		return err
	}

	err = s.templateFiles.WalkPrefix("shared/", func(path string, file packd.File) error {
		layout, err = parseTemplate(layout, file.String())
		return err
	})
	if err != nil {
		return fmt.Errorf("loading shared partial: %w", err)
	}

	templateContent, err := s.templateFiles.FindString(name + ".html")
	if err != nil {
		return errTemplateNotFound
	}

	t, err := parseTemplate(layout, templateContent)
	if err != nil {
		return err
	}

	return t.Execute(w, value)
}

func parseTemplate(layout *template.Template, content string) (*template.Template, error) {
	t, err := layout.Clone()
	if err != nil {
		return nil, err
	}

	_, err = t.New("content").Parse(content)
	return t, err
}

type subTest struct {
	ParentTest *tester.Test
	Test       *tester.Test
	Level      int
	NextLevel  int
}

func (s *UIHandler) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"asSubTest": func(parent *tester.Test, level int, test *tester.Test) subTest {
			return subTest{
				ParentTest: parent,
				Test:       test,
				Level:      level,
				NextLevel:  level + 1,
			}
		},
		"subTestNameIndent": func(level int) int {
			return level * 10
		},
		"trimPrefix": func(prefix, s string) string {
			return strings.TrimPrefix(s, prefix)
		},
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"formatRelativeTime": func(t time.Time) string {
			d := time.Now().Sub(t)
			var suffix string
			if d > 0 {
				suffix = "ago"
			} else {
				suffix = "from now"
			}
			return fmt.Sprintf("%s %s", d.Round(time.Second).String(), suffix)
		},
		"formatDuration": func(d time.Duration) string {
			if d < 1*time.Millisecond {
				return d.Round(time.Microsecond).String()
			}
			if d < 1*time.Minute {
				return d.Round(time.Millisecond).String()
			}
			return d.Round(time.Second).String()
		},
		"formatLogs": func(logData []byte) string {
			return string(logData)
		},
		"testStateMessage": func(state tester.TBState) string {
			switch state {
			case tester.TBPassed:
				return "passed"
			case tester.TBFailed:
				return "failed"
			case tester.TBSkipped:
				return "skipped"
			default:
				return "unknown"
			}
		},
		"testStateColour": func(state tester.TBState) string {
			switch state {
			case tester.TBPassed:
				return "success"
			case tester.TBFailed:
				return "danger"
			case tester.TBSkipped:
				return "warning"
			default:
				return "unknown"
			}
		},
	}
}
