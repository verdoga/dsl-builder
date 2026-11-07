package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/preview"
)

func actionsTestAt() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) }

func actionsTestIssues(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("owner fixture issues: %v", issues)
	}
}

func actionsTestRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func actionsTestWrite(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func actionsTestSource(t *testing.T, dir string, edit func(*model.Result)) string {
	t.Helper()
	raw := actionsTestRead(t, filepath.Join("..", "golden", "2. parsed.json"))
	if edit != nil {
		var result model.Result
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		edit(&result)
		var err error
		raw, err = json.MarshalIndent(result, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "source.json")
	actionsTestWrite(t, path, raw)
	return path
}

func actionsTestCreate(t *testing.T) (*app.Operation, string, string) {
	t.Helper()
	base := t.TempDir()
	sourceDir := filepath.Join(base, "sources")
	if err := os.Mkdir(sourceDir, 0700); err != nil {
		t.Fatal(err)
	}
	source := actionsTestSource(t, sourceDir, nil)
	op, issues := app.NewCreate(base, sourceDir, "Исходный курс", "")
	actionsTestIssues(t, issues)
	actionsTestIssues(t, op.Analyze(actionsTestAt()))
	if s := op.Snapshot(); s.Catalog == nil || !s.ManifestReady || !s.Actions.EditCatalog {
		t.Fatal("Create fixture not ready")
	}
	t.Cleanup(op.Cancel)
	return op, base, source
}

func actionsTestSave(t *testing.T, op *app.Operation, base string) string {
	t.Helper()
	actionsTestIssues(t, op.SetDestination(op.Snapshot().Token, base, "baseline"))
	value, issues := op.ReviewSave(op.Snapshot().Token)
	actionsTestIssues(t, issues)
	if value == nil {
		t.Fatal("nil SavePreview")
	}
	actionsTestIssues(t, op.ConfirmSave(value.Token, true, false))
	result, issues := op.Save(actionsTestAt())
	actionsTestIssues(t, issues)
	if !result.Installed {
		t.Fatal("baseline not installed")
	}
	return result.TargetPath
}

func actionsTestUpdate(t *testing.T, edit func(*model.Result)) (*app.Operation, string, string) {
	t.Helper()
	create, base, _ := actionsTestCreate(t)
	archive := actionsTestSave(t, create, base)
	incoming := filepath.Join(base, "incoming")
	if err := os.Mkdir(incoming, 0700); err != nil {
		t.Fatal(err)
	}
	source := actionsTestSource(t, incoming, edit)
	op, issues := app.NewUpdate(base, archive, source)
	actionsTestIssues(t, issues)
	actionsTestIssues(t, op.Analyze(actionsTestAt().Add(time.Hour)))
	s := op.Snapshot()
	if s.Mode != app.Update || s.Plan == nil || s.RevisionReady {
		t.Fatal("Update fixture not awaiting decisions/revision")
	}
	t.Cleanup(op.Cancel)
	return op, archive, source
}

func actionsTestCourse(t *testing.T, op *app.Operation, title string) {
	t.Helper()
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, title)
	actionsTestIssues(t, issues)
	if edit == nil {
		t.Fatal("nil course edit")
	}
	actionsTestIssues(t, op.AcceptEdit(edit.Token))
}

func actionsTestShown(t *testing.T, text string, s app.Snapshot) {
	t.Helper()
	blocks := []string{}
	if s.Catalog != nil && s.Module != nil {
		composition, err := preview.Composition(s.Catalog, s.Module, s.NewFileNames)
		if err != nil {
			t.Fatal(err)
		}
		blocks = append(blocks, composition, preview.OrderConflicts(s.Conflicts))
	}
	manifest, err := preview.ManifestChanges(s.ManifestChanges)
	if err != nil {
		t.Fatal(err)
	}
	blocks = append(blocks, manifest)
	if s.Actions.UnusedResources {
		blocks = append(blocks, preview.Resources(s.Resources))
	}
	if s.Mode == app.Update && s.RevisionReady {
		heading, err := preview.UpdateHeading(s.ArchivePath, s.Before, s.After)
		if err != nil {
			t.Fatal(err)
		}
		fields, err := preview.AcceptedFields(s.Before, s.After, s.AcceptedFields)
		if err != nil {
			t.Fatal(err)
		}
		replacements, err := preview.Replacements(s.Before, s.After, s.Plan)
		if err != nil {
			t.Fatal(err)
		}
		blocks = append(blocks, heading, fields, replacements)
	}
	for _, block := range blocks {
		if !strings.Contains(text, block) {
			t.Errorf("missing current preview block %q in\n%s", block, text)
		}
	}
}

// actionsTestInput выдаёт по одному ответу после наблюдения реального вывода.
// Это адаптер io.Reader, не модель приложения или копия диалога.
type actionsTestInput struct {
	t       *testing.T
	output  *bytes.Buffer
	steps   []func(string) string
	cursor  int
	reads   int
	pending string
	fault   error
}

func (r *actionsTestInput) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		if r.reads >= len(r.steps) {
			if r.fault != nil {
				return 0, r.fault
			}
			r.t.Errorf("unexpected input after %d answers; output:\n%s", r.reads, r.output.String())
			return 0, io.EOF
		}
		text := r.output.String()[r.cursor:]
		r.cursor = r.output.Len()
		step := r.steps[r.reads]
		r.reads++
		r.pending = step(text) + "\n"
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func actionsTestNumber(t *testing.T, text, label string) string {
	t.Helper()
	found := ""
	for _, line := range strings.Split(text, "\n") {
		number, value, ok := strings.Cut(strings.TrimSpace(line), ". ")
		if !ok || value != label {
			continue
		}
		n, err := strconv.Atoi(number)
		if err != nil || n < 1 {
			continue
		}
		if found != "" {
			t.Fatalf("duplicate label %q in current screen:\n%s", label, text)
		}
		found = number
	}
	if found == "" {
		t.Fatalf("label %q missing in current screen:\n%s", label, text)
	}
	return found
}

func actionsTestPick(t *testing.T, a action, check func(string)) func(string) string {
	t.Helper()
	return func(text string) string {
		if check != nil {
			check(text)
		}
		return actionsTestNumber(t, text, actionLabel(a))
	}
}

func actionsTestAnswer(answer string) func(string) string {
	return func(string) string { return answer }
}

func actionsTestLoop(t *testing.T, op *app.Operation, steps ...func(string) string) (route, string) {
	t.Helper()
	var output bytes.Buffer
	input := &actionsTestInput{t: t, output: &output, steps: steps}
	d := dialog{input: bufio.NewReader(input), output: &output, operation: op}
	got, err := d.operationLoop()
	if err != nil {
		t.Fatalf("operationLoop: %v\n%s", err, output.String())
	}
	if input.reads != len(steps) {
		t.Fatalf("read %d/%d answers\n%s", input.reads, len(steps), output.String())
	}
	return got, output.String()
}

func actionsTestUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	if !bytes.Equal(before, actionsTestRead(t, path)) {
		t.Errorf("source changed: %s", path)
	}
}

func actionsTestNoResult(t *testing.T, op *app.Operation) {
	t.Helper()
	if s := op.Snapshot(); s.WriteResult != nil || s.Actions.Save {
		t.Fatalf("unexpected write/consent: %+v", s.WriteResult)
	}
}

func actionsTestRequireMenu(t *testing.T, text string, values ...action) {
	t.Helper()
	for _, a := range values {
		_ = actionsTestNumber(t, text, actionLabel(a))
	}
}

func actionsTestRequireText(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, value := range fragments {
		if !strings.Contains(text, value) {
			t.Errorf("missing %q in\n%s", value, text)
		}
	}
}

func actionsTestSnapshotDescription(s app.Snapshot) string {
	return fmt.Sprintf("ready=%v noChanges=%v fields=%v placement=%v", s.RevisionReady, s.NoChanges, s.FieldAnswered, s.PlacementAnswered)
}
