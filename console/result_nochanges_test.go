package console

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestResultNoChangesRequiresExplicitPreparedRevision(t *testing.T) {
	op, source, archive := resultTestUpdate(t)
	sourceBefore, archiveBefore := resultTestBytes(t, source), resultTestBytes(t, archive)
	before := op.Snapshot()
	if before.RevisionReady || !before.Actions.PrepareRevision {
		t.Fatal("owner fixture does not await explicit revision preparation")
	}
	// Проверяется защитная проекция: даже ошибочное раннее NoChanges
	// не может заменить обязательную готовность итоговой ревизии.
	before.NoChanges = true
	var output bytes.Buffer
	input := strings.NewReader("2\n")
	d := resultTestDialog(t, input, &output, op)
	got, err := d.noChangesChoice(before)
	if err != nil || got != routePreview || input.Len() != 2 {
		t.Fatalf("premature terminal: route=%v, err=%v, input=%d, output=%s", got, err, input.Len(), output.String())
	}
	resultTestNoTerminal(t, output.String())
	if op.Snapshot().RevisionReady || op.Snapshot().Token != before.Token {
		t.Fatal("noChangesChoice prepared revision or canceled the current operation")
	}
	resultTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
	ready := op.Snapshot()
	if !ready.NoChanges || !ready.RevisionReady || ready.Actions.ReviewSave || ready.Actions.Save || len(ready.Blockers) != 0 {
		t.Fatal("owner fixture did not reach prepared no-change state")
	}
	if err := d.showSnapshot(ready); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	got, err = d.noChangesChoice(ready)
	if err != nil || got != routeExit {
		t.Fatalf("prepared terminal: route=%v, err=%v", got, err)
	}
	if !strings.Contains(strings.ToLower(output.String()), "изменени") || !strings.Contains(strings.ToLower(output.String()), "запис") && !strings.Contains(strings.ToLower(output.String()), "архив") {
		t.Errorf("missing no-change/no-write explanation: %s", output.String())
	}
	resultTestMenu(t, output.String(), "main", "exit")
	resultTestCanceled(t, op, ready.Token)
	if op.Snapshot().WriteResult != nil {
		t.Error("no-change terminal attempted a file write")
	}
	resultTestUnchanged(t, source, sourceBefore)
	resultTestUnchanged(t, archive, archiveBefore)
}

func TestResultNoChangesRejectsIncompleteOrBlockedSnapshots(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(*app.Snapshot)
	}{
		{"has_changes", func(s *app.Snapshot) { s.NoChanges = false }},
		{"blocking_issue", func(s *app.Snapshot) {
			s.Blockers = []*diagnostic.Error{{Code: "catalog.weight", Reason: "Нужно исправить вес"}}
		}},
		{"pending_field", func(s *app.Snapshot) {
			s.Plan = &compare.Plan{Fields: []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1}}}
			s.FieldAnswered = []bool{false}
			s.Actions.DecideFields = true
		}},
		{"missing_field_answers", func(s *app.Snapshot) {
			s.Plan = &compare.Plan{Fields: []compare.FieldChange{{Field: compare.PageTitle, OldPage: -1, NewPage: 0}}}
			s.FieldAnswered = nil
			s.Actions.DecideFields = true
		}},
		{"pending_placement", func(s *app.Snapshot) {
			s.PlacementProposals = []catalog.Change{{FileName: "l001.json", Field: "order"}}
			s.PlacementAnswered = []bool{false}
			s.Actions.DecidePlacement = true
		}},
		{"missing_placement_answers", func(s *app.Snapshot) {
			s.PlacementProposals = []catalog.Change{{FileName: "l001.json", Field: "section"}}
			s.PlacementAnswered = nil
			s.Actions.DecidePlacement = true
		}},
		{"revision_not_ready_after_rejections", func(s *app.Snapshot) {
			s.RevisionReady = false
			s.Plan = &compare.Plan{Fields: []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1}}}
			s.FieldAnswered, s.FieldAccepted = []bool{true}, []bool{false}
			s.Actions.PrepareRevision = true
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			op, _, _ := resultTestUpdate(t)
			resultTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
			snapshot := op.Snapshot()
			test.alter(&snapshot)
			var output bytes.Buffer
			input := strings.NewReader("1\n")
			d := resultTestDialog(t, input, &output, op)
			got, err := d.noChangesChoice(snapshot)
			if err != nil || got != routePreview || input.Len() != 2 {
				t.Fatalf("invalid no-change state consumed terminal choice: route=%v, err=%v, input=%d, output=%s", got, err, input.Len(), output.String())
			}
			if op.Snapshot().Token != snapshot.Token || op.Snapshot().WriteResult != nil {
				t.Error("guard changed/canceled operation or wrote archive")
			}
			resultTestNoTerminal(t, output.String())
		})
	}
}

func TestResultNoChangesDoesNotReplaceOutstandingContentChanges(t *testing.T) {
	_, source, archive := resultTestSaved(t)
	raw := resultTestBytes(t, source)
	// Меняется технический JSON до Analyze; загруженные Input не правятся.
	changed := bytes.ReplaceAll(raw, []byte("EXAMPLE TEXT"), []byte("CHANGED TEXT"))
	resultTestWriteSource(t, source, changed)
	op, issues := app.NewUpdate(filepath.Dir(archive), archive, source)
	resultTestIssues(t, issues)
	resultTestIssues(t, op.Analyze(resultTestTime(t)))
	snapshot := op.Snapshot()
	if snapshot.RevisionReady || !snapshot.Actions.PrepareRevision || snapshot.Plan == nil || len(snapshot.Plan.Pages) == 0 {
		t.Fatal("fixture did not produce pending content revision")
	}
	var output bytes.Buffer
	d := resultTestDialog(t, strings.NewReader("2\n"), &output, op)
	got, err := d.noChangesChoice(snapshot)
	if err != nil || got != routePreview || op.Snapshot().RevisionReady {
		t.Fatalf("content update terminated before preparation: route=%v err=%v text=%s", got, err, output.String())
	}
	resultTestNoTerminal(t, output.String())
}

func TestResultNoChangesAfterRejectedFieldStillRequiresPreparation(t *testing.T) {
	_, source, archive := resultTestSaved(t)
	raw := resultTestBytes(t, source)
	raw = bytes.Replace(raw, []byte(`"title": "TITLE"`), []byte(`"title": "OTHER"`), 1)
	raw = bytes.Replace(raw, []byte(`# TITLE`), []byte(`# OTHER`), 1)
	raw = bytes.Replace(raw, []byte(`"raw": "TITLE"`), []byte(`"raw": "OTHER"`), 1)
	raw = bytes.Replace(raw, []byte(`"value": "TITLE"`), []byte(`"value": "OTHER"`), 1)
	resultTestWriteSource(t, source, raw)
	op, issues := app.NewUpdate(filepath.Dir(archive), archive, source)
	resultTestIssues(t, issues)
	resultTestIssues(t, op.Analyze(resultTestTime(t)))
	snapshot := op.Snapshot()
	if snapshot.Plan == nil || len(snapshot.Plan.Fields) != 1 || len(snapshot.Plan.Pages) != 0 {
		t.Fatal("fixture is not an isolated title proposal")
	}
	resultTestIssues(t, op.DecideField(snapshot.Token, 0, false))
	snapshot = op.Snapshot()
	if snapshot.RevisionReady || !snapshot.FieldAnswered[0] || snapshot.FieldAccepted[0] || !snapshot.Actions.PrepareRevision {
		t.Fatal("fixture did not preserve a rejected field awaiting preparation")
	}
	var output bytes.Buffer
	input := strings.NewReader("1\n")
	d := resultTestDialog(t, input, &output, op)
	got, err := d.noChangesChoice(snapshot)
	if err != nil || got != routePreview || input.Len() != 2 || op.Snapshot().RevisionReady {
		t.Fatalf("rejected title ended operation before preparation: route=%v error=%v", got, err)
	}
	resultTestNoTerminal(t, output.String())
	resultTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
	ready := op.Snapshot()
	if !ready.NoChanges || !ready.RevisionReady || ready.Actions.ReviewSave || ready.Actions.Save {
		t.Fatal("prepared rejected title did not yield owner no-change state")
	}
	if err := d.showSnapshot(ready); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	got, err = d.noChangesChoice(ready)
	if err != nil || got != routeMain || op.Snapshot().WriteResult != nil {
		t.Fatalf("prepared declined title route=%v error=%v", got, err)
	}
	resultTestCanceled(t, op, ready.Token)
	resultTestUnchanged(t, source, raw)
}

func TestResultNoChangesStaleSnapshotCannotTerminateChangedOperation(t *testing.T) {
	op, _, _ := resultTestUpdate(t)
	resultTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
	stale := op.Snapshot()
	var output bytes.Buffer
	d := resultTestDialog(t, strings.NewReader("2\n"), &output, op)
	if err := d.showSnapshot(stale); err != nil {
		t.Fatal(err)
	}
	edit, issues := op.ProposeCourseTitle(stale.Token, "Другой курс")
	resultTestIssues(t, issues)
	if edit == nil {
		t.Fatal("owner fixture did not create catalog edit")
	}
	resultTestIssues(t, op.AcceptEdit(edit.Token))
	current := op.Snapshot()
	if current.NoChanges || current.Token == stale.Token {
		t.Fatal("owner fixture did not invalidate old no-change state")
	}
	output.Reset()
	got, err := d.noChangesChoice(stale)
	if err != nil || got != routePreview || op.Snapshot().Token != current.Token {
		t.Fatalf("stale snapshot terminated current changed operation: route=%v error=%v", got, err)
	}
	resultTestNoTerminal(t, output.String())
}

func TestResultNoChangesAllowsCompletedPlacementAnswers(t *testing.T) {
	op, _, archive := resultTestUpdate(t)
	resultTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
	snapshot := op.Snapshot()
	// Независимая проекция уже отклонённых предложений: принятый каталог
	// остаётся прежним, оба отдельных факта ответа завершены.
	snapshot.PlacementProposals = []catalog.Change{
		{FileName: "l001.json", Field: "section", Before: json.RawMessage(`null`), After: json.RawMessage(`"Новая секция"`)},
		{FileName: "l001.json", Field: "order", Before: json.RawMessage(`"1"`), After: json.RawMessage(`"2"`)},
	}
	snapshot.PlacementAnswered = []bool{true, true}
	before := resultTestBytes(t, archive)
	var output bytes.Buffer
	d := resultTestDialog(t, strings.NewReader("1\n"), &output, op)
	if err := d.showSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	got, err := d.noChangesChoice(snapshot)
	if err != nil || got != routeMain || op.Snapshot().WriteResult != nil {
		t.Fatalf("completed placement decisions did not permit no-change terminal: route=%v error=%v", got, err)
	}
	resultTestCanceled(t, op, snapshot.Token)
	resultTestUnchanged(t, archive, before)
}
