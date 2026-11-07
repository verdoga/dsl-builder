package console

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/moduleio"
)

type runTestScriptReader struct {
	steps   []func() string
	pending string
	calls   int
}

func (r *runTestScriptReader) Read(p []byte) (int, error) {
	if r.pending == "" {
		if len(r.steps) == 0 {
			return 0, io.EOF
		}
		next := r.steps[0]
		r.steps = r.steps[1:]
		r.calls++
		r.pending = next()
		if r.pending == "" {
			return 0, io.EOF
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
func runTestReply(text string) func() string { return func() string { return text } }

func runTestMenuNumber(t *testing.T, output, label string) string {
	t.Helper()
	lines := strings.Split(output, "\n")
	pattern := regexp.MustCompile(`^\s*([0-9]+)\.\s+(.*?)\s*$`)
	for i := len(lines) - 1; i >= 0; i-- {
		match := pattern.FindStringSubmatch(lines[i])
		if len(match) == 3 && match[2] == label {
			return match[1] + "\n"
		}
	}
	t.Fatalf("menu label %q absent from output %q", label, output)
	return ""
}

func runTestSource(t *testing.T, directory, id string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "run", "source.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.ReplaceAll(raw, []byte("BA82"), []byte(id))
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "урок с пробелом.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runTestArchive(t *testing.T, base, sourceDirectory string) string {
	t.Helper()
	op, issues := app.NewCreate(base, sourceDirectory, "Курс архива", "")
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	defer op.Cancel()
	issues = op.Analyze(time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC))
	if len(issues) != 0 {
		t.Fatalf("fixture analyze: %v", issues)
	}
	snapshot := op.Snapshot()
	if !snapshot.ManifestReady || snapshot.Module == nil {
		t.Fatal("fixture module not ready")
	}
	var zip bytes.Buffer
	path := filepath.Join(base, "исходный модуль.zip")
	if issues := moduleio.Write(&zip, snapshot.Module, diagnostic.Context{Path: path}); len(issues) != 0 {
		t.Fatal(issues)
	}
	if err := os.WriteFile(path, zip.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if issues := moduleio.Verify(path); len(issues) != 0 {
		t.Fatal(issues)
	}
	return path
}

func TestRunInitialModesAnalyzeRealOwnersThenCancel(t *testing.T) {
	for _, mode := range []app.Mode{app.Create, app.Update, app.Add} {
		t.Run(string(mode), func(t *testing.T) {
			base := t.TempDir()
			source := runTestSource(t, filepath.Join(base, "входные JSON"), "BA82")
			archive := runTestArchive(t, base, filepath.Dir(source))
			if mode == app.Add {
				source = runTestSource(t, filepath.Join(base, "добавление"), "BA83")
			}
			preserved := map[string][]byte{}
			for _, path := range []string{source, archive} {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				preserved[path] = raw
			}
			var out bytes.Buffer
			in := &runTestScriptReader{}
			d := dialog{input: bufio.NewReader(in), output: &out, baseDir: base}
			course := "Курс 日本語 🙂 " + strings.Repeat("Ю", 70000)
			number := map[app.Mode]string{app.Create: "1\n", app.Update: "2\n", app.Add: "3\n"}[mode]
			in.steps = append(in.steps, runTestReply(number))
			if mode == app.Create {
				in.steps = append(in.steps, runTestReply("1\n\""+filepath.Dir(source)+"\"\n1\n"+course+"\n1\n\n"))
			} else {
				in.steps = append(in.steps, runTestReply("1\n\""+archive+"\"\n1\n\""+source+"\"\n"))
			}
			var active *app.Operation
			var token app.ReviewToken
			earliest := time.Now().Add(-time.Second)
			in.steps = append(in.steps, func() string {
				active = d.operation
				if active == nil {
					t.Fatal("complete initial group did not create Operation")
				}
				snapshot := active.Snapshot()
				token = snapshot.Token
				if snapshot.Mode != mode || snapshot.Catalog == nil || snapshot.Module == nil {
					t.Fatalf("initial analysis not available: mode=%s blockers=%v", snapshot.Mode, snapshot.Blockers)
				}
				if mode == app.Create {
					if snapshot.Catalog.CourseTitle != course || snapshot.Catalog.UnitTitle != nil {
						t.Fatal("Unicode/long course or optional unit changed")
					}
					at, err := time.Parse(time.RFC3339, snapshot.Module.Manifest.CreatedAt)
					if err != nil || at.Before(earliest) || at.After(time.Now().Add(time.Second)) {
						t.Fatalf("Analyze did not receive current time: %q", snapshot.Module.Manifest.CreatedAt)
					}
				}
				if mode == app.Update && snapshot.RevisionReady {
					t.Error("revision prepared before explicit action")
				}
				if mode == app.Add && (len(snapshot.Catalog.Items) != 2 || len(snapshot.NewFileNames) != 1) {
					t.Fatal("new and previous lessons not retained")
				}
				return runTestMenuNumber(t, out.String(), actionLabel(actionExit))
			})
			if err := d.mainLoop(); err != nil {
				t.Fatal(err)
			}
			if active == nil || active.Snapshot().Token == token || active.Snapshot().Actions.Cancel {
				t.Fatal("exit did not cancel analyzed operation")
			}
			if len(in.steps) != 0 {
				t.Fatal("script was not consumed")
			}
			for path, before := range preserved {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Errorf("input changed: %s", path)
				}
			}
		})
	}
}

func TestRunModeNavigationRestartsWithNewOperationAndReturnsMain(t *testing.T) {
	base := t.TempDir()
	source := runTestSource(t, filepath.Join(base, "source"), "BA82")
	var out bytes.Buffer
	in := &runTestScriptReader{}
	d := dialog{input: bufio.NewReader(in), output: &out, baseDir: base}
	group := func(course string) string { return "1\n" + filepath.Dir(source) + "\n1\n" + course + "\n1\n\n" }
	var first, second *app.Operation
	var firstToken, secondToken app.ReviewToken
	in.steps = []func() string{
		runTestReply("1\n"), runTestReply(group("Первый курс")),
		func() string {
			first = d.operation
			if first == nil {
				t.Fatal("first operation missing")
			}
			firstToken = first.Snapshot().Token
			return runTestMenuNumber(t, out.String(), actionLabel(actionRestart))
		},
		func() string {
			if first.Snapshot().Token == firstToken {
				t.Error("restart did not cancel old operation")
			}
			return group("Другой курс")
		},
		func() string {
			second = d.operation
			if second == nil || second == first {
				t.Fatal("restart reused operation")
			}
			secondToken = second.Snapshot().Token
			if second.Snapshot().Catalog.CourseTitle != "Другой курс" {
				t.Error("old initial data reused")
			}
			return runTestMenuNumber(t, out.String(), actionLabel(actionMain))
		},
		func() string {
			if second.Snapshot().Token == secondToken {
				t.Error("main route did not cancel")
			}
			return "4\n"
		},
	}
	if err := d.mainLoop(); err != nil {
		t.Fatal(err)
	}
	if len(in.steps) != 0 {
		t.Fatalf("remaining steps=%d, reads=%s", len(in.steps), strconv.Itoa(in.calls))
	}
	if got := strings.Count(out.String(), "1. Создать новый модуль"); got != 2 {
		t.Errorf("main menu shown %d times; want 2", got)
	}
}
