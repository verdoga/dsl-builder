package console

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/preview"
	"github.com/verdoga/dsl-builder/revision"
)

// consoleUpdateTestMenuAnswer использует номер фактически показанного пункта.
func consoleUpdateTestMenuAnswer(t *testing.T, output string, selected action) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^([0-9]+)\. ` + regexp.QuoteMeta(actionLabel(selected)) + `$`)
	matches := pattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		t.Fatalf("action %q is absent from the shown menu: %q", selected, output)
	}
	return matches[len(matches)-1][1] + "\n"
}

func TestConsoleUpdateLoopFreshMenuBetweenSeparateFields(t *testing.T) {
	before := consoleUpdateTestPlain()
	after := consoleUpdateTestResult("Новое", consoleUpdateTestText("Подзаголовок"), []*string{nil}, [][]string{{"Материал", "Второй"}})
	op := consoleUpdateTestOperation(t, before, after, false)
	consoleUpdateTestFieldCount(t, op.Snapshot(), 2)
	var out bytes.Buffer
	stage, offset := 0, 0
	reader := &consoleUpdateTestReader{next: func() (string, error) {
		current := stage
		stage++
		snapshot := op.Snapshot()
		screen := out.String()[offset:]
		switch current {
		case 0:
			offset = out.Len()
			return consoleUpdateTestMenuAnswer(t, screen, actionField), nil
		case 1:
			consoleUpdateTestFieldProposal(t, snapshot, 0, screen)
			offset = out.Len()
			return "2\n", nil
		case 2:
			if !snapshot.FieldAnswered[0] || snapshot.FieldAnswered[1] || !snapshot.Actions.DecideFields {
				t.Fatal("loop skipped fresh menu after one separate field decision")
			}
			offset = out.Len()
			return consoleUpdateTestMenuAnswer(t, screen, actionField), nil
		case 3:
			consoleUpdateTestFieldProposal(t, snapshot, 1, screen)
			offset = out.Len()
			return "1\n", nil
		case 4:
			if !snapshot.FieldAnswered[1] || !snapshot.FieldAccepted[1] || snapshot.FieldAccepted[0] ||
				snapshot.RevisionReady || !snapshot.Actions.PrepareRevision {
				t.Fatal("loop reused old token or prepared revision before explicit selection")
			}
			return consoleUpdateTestMenuAnswer(t, screen, actionMain), nil
		default:
			return "", io.EOF
		}
	}}
	d := &dialog{operation: op, input: bufio.NewReader(reader), output: &out}
	where, err := d.operationLoop()
	if err != nil || where != routeMain || stage != 5 || op.Snapshot().RevisionReady || op.Snapshot().WriteResult != nil {
		t.Fatalf("unexpected interaction outcome: route=%v err=%v stages=%d", where, err, stage)
	}
}

func TestConsoleUpdateLoopShowsFreshPreparedSnapshotBeforeNoChanges(t *testing.T) {
	before := consoleUpdateTestPlain()
	after := consoleUpdateTestResult("Новое", nil, []*string{nil}, [][]string{{"Материал", "Второй"}})
	op := consoleUpdateTestOperation(t, before, after, false)
	var out bytes.Buffer
	stage, revisionOffset := 0, 0
	reader := &consoleUpdateTestReader{next: func() (string, error) {
		current := stage
		stage++
		snapshot := op.Snapshot()
		switch current {
		case 0:
			if snapshot.RevisionReady || snapshot.NoChanges {
				t.Fatal("loop prepared revision before any explicit action")
			}
			return consoleUpdateTestMenuAnswer(t, out.String(), actionField), nil
		case 1:
			return "2\n", nil
		case 2:
			if snapshot.RevisionReady || snapshot.NoChanges || !snapshot.Actions.PrepareRevision {
				t.Fatal("field refusal became terminal without explicit preparation")
			}
			revisionOffset = out.Len()
			return consoleUpdateTestMenuAnswer(t, out.String(), actionRevision), nil
		case 3:
			if !snapshot.RevisionReady || !snapshot.NoChanges || snapshot.Actions.Save {
				t.Fatal("terminal question preceded a ready owner snapshot")
			}
			composition, err := preview.Composition(snapshot.Catalog, snapshot.Module, snapshot.NewFileNames)
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := preview.ManifestChanges(snapshot.ManifestChanges)
			if err != nil {
				t.Fatal(err)
			}
			screen := out.String()[revisionOffset:]
			terminal := strings.Index(screen, "Содержательных изменений не обнаружено")
			for _, block := range []string{composition, manifest} {
				position := strings.Index(screen, block)
				if position < 0 || terminal < position+len(block) {
					t.Fatalf("fresh complete snapshot must be shown before no-changes terminal: %q", screen)
				}
			}
			return "2\n", nil // Контракт terminalChoice: 1 — главное меню, 2 — выход.
		default:
			return "", io.EOF
		}
	}}
	d := &dialog{operation: op, input: bufio.NewReader(reader), output: &out}
	where, err := d.operationLoop()
	if err != nil || where != routeExit || stage != 4 || op.Snapshot().WriteResult != nil {
		t.Fatalf("unexpected no-change loop outcome: %v %v stages=%d", where, err, stage)
	}
}

func TestConsoleUpdateLoopOwnerFailureShowsFreshSnapshotWithoutRetry(t *testing.T) {
	before := consoleUpdateTestPlain()
	after := consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Новый материал", "Второй"}})
	op := consoleUpdateTestOperation(t, before, after, false, true)
	var out bytes.Buffer
	stage, offset := 0, 0
	reader := &consoleUpdateTestReader{next: func() (string, error) {
		current := stage
		stage++
		snapshot := op.Snapshot()
		switch current {
		case 0:
			if snapshot.RevisionReady || !snapshot.Actions.PrepareRevision {
				t.Fatal("fixture must expose one explicit revision action")
			}
			offset = out.Len()
			return consoleUpdateTestMenuAnswer(t, out.String(), actionRevision), nil
		case 1:
			screen := out.String()[offset:]
			if snapshot.RevisionReady || !snapshot.Actions.PrepareRevision ||
				strings.Count(screen, revision.CodeHistory) != 1 || strings.Contains(screen, "Содержательных изменений не обнаружено") {
				t.Fatalf("owner failure was retried automatically or treated as terminal: %q", screen)
			}
			composition, err := preview.Composition(snapshot.Catalog, snapshot.Module, snapshot.NewFileNames)
			if err != nil || !strings.Contains(screen, composition) {
				t.Fatalf("fresh snapshot was not shown after owner failure: %v %q", err, screen)
			}
			return consoleUpdateTestMenuAnswer(t, screen, actionMain), nil
		default:
			return "", io.EOF
		}
	}}
	d := &dialog{operation: op, input: bufio.NewReader(reader), output: &out}
	where, err := d.operationLoop()
	if err != nil || where != routeMain || stage != 2 || op.Snapshot().WriteResult != nil {
		t.Fatalf("unexpected owner-failure loop outcome: %v %v stages=%d", where, err, stage)
	}
}
