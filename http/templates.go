package http

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/markbates/pkger"
	"github.com/nanzhong/tester"
)

type errTemplateNotFound struct {
	path string
}

func init() {
	pkger.Include("/http/templates")
}

func (e *errTemplateNotFound) Error() string {
	return fmt.Sprintf("template not found: %s", e.path)
}

type errTemplateInvalid struct {
	path string
}

func (e *errTemplateInvalid) Error() string {
	return fmt.Sprintf("template invalid: %s", e.path)
}

// ExecuteTemplate runs the given template with the value
func (s *UIHandler) ExecuteTemplate(name string, w io.Writer, value interface{}) error {
	defaultLayoutPath := "/http/templates/layouts/default.html"
	file, err := pkger.Open(defaultLayoutPath)
	if err != nil {
		return &errTemplateNotFound{defaultLayoutPath}
	}
	layoutContent, err := ioutil.ReadAll(file)
	if err != nil {
		return &errTemplateInvalid{defaultLayoutPath}
	}

	layout, err := template.New("layout_default").Funcs(s.templateFuncs()).Parse(string(layoutContent))
	if err != nil {
		return err
	}

	err = pkger.Walk("/http/templates/shared", func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fileInfo.IsDir() {
			return nil
		}

		file, err := pkger.Open(path)
		if err != nil {
			return &errTemplateNotFound{path}
		}
		templateData, err := ioutil.ReadAll(file)
		if err != nil {
			return &errTemplateInvalid{defaultLayoutPath}
		}

		layout, err = parseTemplate(layout, string(templateData))
		return err
	})
	if err != nil {
		return fmt.Errorf("loading shared partial: %w", err)
	}

	templatePath := "/http/templates/" + name + ".html"
	file, err = pkger.Open(templatePath)
	if err != nil {
		return &errTemplateNotFound{templatePath}
	}
	templateData, err := ioutil.ReadAll(file)
	if err != nil {
		return &errTemplateInvalid{templatePath}
	}

	t, err := parseTemplate(layout, string(templateData))
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
	ParentTest *tester.T
	Test       *tester.T
	Level      int
	NextLevel  int
}

func (s *UIHandler) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"asSubTest": func(parent *tester.T, level int, test *tester.T) subTest {
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
		"formatLogs": func(logData []tester.TBLog) string {
			var b strings.Builder
			for _, l := range logData {
				b.Write(l.Output)
			}
			return b.String()
		},
		"testStateMessage": func(state tester.TBState) string {
			return string(state)
		},
		"testStateColour": func(state tester.TBState) string {
			switch state {
			case tester.TBStatePassed:
				return "success"
			case tester.TBStateFailed:
				return "danger"
			case tester.TBStateSkipped:
				return "warning"
			default:
				return "unknown"
			}
		},
		"runState": func(run *tester.Run) string {
			if run.StartedAt.IsZero() {
				return "pending"
			}
			if run.FinishedAt.IsZero() {
				return "running"
			}
			if run.Error == "" {
				return "passed"
			}
			return "failed"
		},
		"runTests": func(run *tester.Run) int {
			return len(run.Tests)
		},
		"runTestsPassed": func(run *tester.Run) int {
			num := 0
			for _, t := range run.Tests {
				if t.Result.State == tester.TBStatePassed {
					num++
				}
			}
			return num
		},
		"runTestsPassedPercent": func(run *tester.Run) float64 {
			num := 0
			for _, t := range run.Tests {
				if t.Result.State == tester.TBStatePassed {
					num++
				}
			}
			return float64(num) / float64(len(run.Tests)) * 100
		},
		"runTestsSkipped": func(run *tester.Run) int {
			num := 0
			for _, t := range run.Tests {
				if t.Result.State == tester.TBStateSkipped {
					num++
				}
			}
			return num
		},
		"runTestsSkippedPercent": func(run *tester.Run) float64 {
			num := 0
			for _, t := range run.Tests {
				if t.Result.State == tester.TBStateSkipped {
					num++
				}
			}
			return float64(num) / float64(len(run.Tests)) * 100
		},
		"runTestsFailed": func(run *tester.Run) int {
			num := 0
			for _, t := range run.Tests {
				if t.Result.State == tester.TBStateFailed {
					num++
				}
			}
			return num
		},
		"runTestsFailedPercent": func(run *tester.Run) float64 {
			num := 0
			for _, t := range run.Tests {
				if t.Result.State == tester.TBStateFailed {
					num++
				}
			}
			return float64(num) / float64(len(run.Tests)) * 100
		},
	}
}
