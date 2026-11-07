package console

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/preview"
)

func consoleSaveTestBlocks(t *testing.T, s app.Snapshot) []string {
	t.Helper()
	composition, err := preview.Composition(s.Catalog, s.Module, s.NewFileNames)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := preview.ManifestChanges(s.ManifestChanges)
	if err != nil {
		t.Fatal(err)
	}
	blocks := []string{composition, preview.OrderConflicts(s.Conflicts), manifest}
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
	return blocks
}

func TestConsoleSaveReviewShowsEntirePlanBeforeFirstQuestion(t *testing.T) {
	for _, mode := range []string{"create", "update"} {
		t.Run(mode, func(t *testing.T) {
			f := consoleSaveTestCreate(t, false)
			if mode == "create" {
				consoleSaveTestTarget(t, f)
			} else {
				f = consoleSaveTestUpdate(t, "Предложенное название", "")
				for index := range f.op.Snapshot().FieldAnswered {
					consoleSaveTestIssues(t, f.op.DecideField(f.op.Snapshot().Token, index, true))
				}
				consoleSaveTestIssues(t, f.op.PrepareRevision(f.op.Snapshot().Token))
			}
			unchanged := consoleSaveTestUnchanged(t, f.target)
			snapshot := f.op.Snapshot()
			blocks := consoleSaveTestBlocks(t, snapshot)
			var out bytes.Buffer
			r := &consoleSaveTestReader{lines: []string{"2\n"}}
			r.before = func(index int) {
				if index != 0 {
					t.Fatal("consent read more than once after refusal")
				}
				question := strings.Index(out.String(), "Записать показанный модуль?")
				if question < 0 {
					t.Fatal("first consent question missing")
				}
				prefix := out.String()[:question]
				for _, block := range blocks {
					if !strings.Contains(prefix, block) {
						t.Errorf("complete owner block absent before consent:\n%s\noutput:\n%s", block, prefix)
					}
				}
				if !strings.Contains(prefix, f.target) || !strings.Contains(strings.ToLower(prefix), "существ") {
					t.Errorf("Destination.Path/Exists not displayed: %s", prefix)
				}
				if !strings.Contains(strings.ToLower(prefix), "манифест") || !strings.Contains(strings.ToLower(prefix), "дат") {
					t.Error("manifest date change not disclosed before consent")
				}
				if f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil {
					t.Error("premature consent/write")
				}
			}
			got, err := consoleSaveTestDialog(f.op, r, &out).reviewSaving(snapshot)
			if err != nil || got != routePreview || r.index != 1 {
				t.Fatalf("route=%v err=%v answers=%d", got, err, r.index)
			}
			unchanged()
		})
	}
}

func TestConsoleSavePreviewUsesSuppliedSnapshotAndIsReadOnly(t *testing.T) {
	f := consoleSaveTestCreate(t, false)
	unchanged := consoleSaveTestUnchanged(t, f.target)
	value := consoleSaveTestPreview(t, f)
	// Публичный DTO представляет уже закрытый показ; Operation не перечитывается.
	edit, issues := f.op.ProposeCourseTitle(f.op.Snapshot().Token, "Другой текущий курс")
	consoleSaveTestIssues(t, issues)
	consoleSaveTestIssues(t, f.op.AcceptEdit(edit.Token))
	before, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	d := consoleSaveTestDialog(f.op, strings.NewReader(""), &out)
	if err := d.showSavePreview(value); err != nil {
		t.Fatal(err)
	}
	for _, block := range consoleSaveTestBlocks(t, value.Snapshot) {
		if !strings.Contains(out.String(), block) {
			t.Errorf("supplied block absent: %s", block)
		}
	}
	if strings.Contains(out.String(), "Другой текущий курс") {
		t.Error("fresh operation state substituted for supplied preview")
	}
	after, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("SavePreview mutated")
	}
	base := out.String()
	copyValue := *value
	copyValue.Destination.Exists = true
	out.Reset()
	if err := d.showSavePreview(&copyValue); err != nil {
		t.Fatal(err)
	}
	if out.String() == base {
		t.Error("Exists=true/false indistinguishable")
	}
	existingText := out.String()
	copyValue.ManifestTimeWillChange = false
	out.Reset()
	if err := d.showSavePreview(&copyValue); err != nil {
		t.Fatal(err)
	}
	if out.String() == existingText {
		t.Error("ManifestTimeWillChange not represented")
	}
	unchanged()
}

func TestConsoleSavePreviewRejectsNilAndEveryBrokenBlock(t *testing.T) {
	for _, block := range []string{"nil", "composition", "manifest", "multiple", "update"} {
		t.Run(block, func(t *testing.T) {
			f := consoleSaveTestCreate(t, false)
			unchanged := consoleSaveTestUnchanged(t, f.target)
			value := consoleSaveTestPreview(t, f)
			before := f.op.Snapshot()
			switch block {
			case "nil":
				value = nil
			case "composition":
				value.Snapshot.Module.Files = nil
			case "manifest":
				value.Snapshot.ManifestChanges = []catalog.Change{{Field: "unknown", After: json.RawMessage(`"secret"`)}}
			case "multiple":
				value.Snapshot.Module.Files = nil
				value.Snapshot.ManifestChanges = []catalog.Change{{Field: "unknown"}}
			case "update":
				value.Snapshot.Mode = app.Update
				value.Snapshot.RevisionReady = true
			}
			var out bytes.Buffer
			r := &consoleSaveTestReader{lines: []string{"1\n"}}
			err := consoleSaveTestDialog(f.op, r, &out).showSavePreview(value)
			if err == nil || !consoleSaveTestHasCode(err, CodePreview) || consoleSaveTestHasCode(err, CodeOutput) {
				t.Fatalf("not a presentation error: %v", err)
			}
			if r.reads != 0 || strings.Contains(out.String(), "Записать показанный модуль?") || f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil {
				t.Error("broken preview asked consent or wrote")
			}
			if block == "multiple" {
				_, composition := preview.Composition(value.Snapshot.Catalog, value.Snapshot.Module, value.Snapshot.NewFileNames)
				_, manifest := preview.ManifestChanges(value.Snapshot.ManifestChanges)
				consoleSaveTestPresentationCauses(t, err, composition, manifest)
			}
			consoleSaveTestSnapshotUnchanged(t, before, f.op.Snapshot())
			unchanged()
		})
	}
}

func TestConsoleSaveReviewUnavailableNeverAsksConsent(t *testing.T) {
	for _, state := range []string{"no-target", "stale", "candidate", "undecided-fields", "undecided-placement", "no-changes", "blockers", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			f := consoleSaveTestCreate(t, false)
			snapshot := f.op.Snapshot()
			switch state {
			case "stale":
				snapshot = consoleSaveTestTarget(t, f)
				consoleSaveTestTarget(t, f)
			case "candidate":
				consoleSaveTestTarget(t, f)
				snapshot = f.op.Snapshot()
				edit, issues := f.op.ProposeCourseTitle(snapshot.Token, "Кандидат")
				consoleSaveTestIssues(t, issues)
				if edit == nil {
					t.Fatal("no candidate")
				}
			case "undecided-fields":
				f = consoleSaveTestUpdate(t, "Новое название", "")
				snapshot = f.op.Snapshot()
				if !snapshot.Actions.DecideFields {
					t.Fatal("no pending fields")
				}
			case "undecided-placement":
				f = consoleSaveTestUpdate(t, "", "20")
				snapshot = f.op.Snapshot()
				if !snapshot.Actions.DecidePlacement {
					t.Fatal("no pending placement")
				}
				consoleSaveTestIssues(t, f.op.PrepareRevision(snapshot.Token))
				snapshot = f.op.Snapshot()
			case "no-changes":
				f = consoleSaveTestUpdate(t, "", "")
				consoleSaveTestIssues(t, f.op.PrepareRevision(f.op.Snapshot().Token))
				snapshot = f.op.Snapshot()
				if !snapshot.NoChanges {
					t.Fatal("fixture not NoChanges")
				}
			case "blockers":
				f = consoleSaveTestUpdate(t, "", "")
				consoleSaveTestIssues(t, f.op.PrepareRevision(f.op.Snapshot().Token))
				edit, _ := f.op.ProposeSection(f.op.Snapshot().Token, "Новая секция")
				if edit == nil {
					t.Fatal("no section candidate")
				}
				f.op.AcceptEdit(edit.Token)
				snapshot = f.op.Snapshot()
				if len(snapshot.Blockers) == 0 {
					t.Fatal("fixture has no blockers")
				}
			case "cancelled":
				f.op.Cancel()
				snapshot = f.op.Snapshot()
			}
			ownerRejected := state == "stale" || state == "candidate"
			if snapshot.Actions.ReviewSave != ownerRejected {
				t.Fatalf("unexpected owner action for %s: %+v", state, snapshot.Actions)
			}
			before := f.op.Snapshot()
			unchanged := consoleSaveTestUnchanged(t, f.target)
			old := []byte(nil)
			if f.old != nil {
				old = f.old.OriginalBytes()
			}
			var out bytes.Buffer
			r := &consoleSaveTestReader{lines: []string{"1\n", "1\n"}}
			got, err := consoleSaveTestDialog(f.op, r, &out).reviewSaving(snapshot)
			if err != nil || got != routePreview {
				t.Fatalf("route=%v err=%v", got, err)
			}
			if r.reads != 0 || strings.Contains(out.String(), "Записать показанный модуль?") || f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil {
				t.Fatalf("unavailable review continued: %s", out.String())
			}
			if ownerRejected && out.Len() == 0 {
				t.Error("owner rejection issues were not shown")
			}
			if state == "stale" && !strings.Contains(out.String(), app.CodeStaleReview) {
				t.Error("stale shown token was not rejected by owner")
			}
			consoleSaveTestSnapshotUnchanged(t, before, f.op.Snapshot())
			if f.old != nil && !bytes.Equal(old, f.old.OriginalBytes()) {
				t.Error("old OriginalBytes mutated")
			}
			unchanged()
		})
	}
}

func TestConsoleSaveReviewNilOperationDoesNotAskConsent(t *testing.T) {
	r := &consoleSaveTestReader{lines: []string{"1\n"}}
	var out bytes.Buffer
	var op *app.Operation
	before := op.Snapshot()
	for _, snapshot := range []app.Snapshot{{}, before} {
		got, err := consoleSaveTestDialog(op, r, &out).reviewSaving(snapshot)
		if err != nil || got != routePreview || r.reads != 0 {
			t.Fatalf("route=%v err=%v reads=%d", got, err, r.reads)
		}
		if strings.Contains(out.String(), "Записать показанный модуль?") || op.Snapshot().Actions.Save || op.Snapshot().WriteResult != nil {
			t.Fatalf("nil preview not handled: %s", out.String())
		}
	}
	consoleSaveTestSnapshotUnchanged(t, before, op.Snapshot())
}
