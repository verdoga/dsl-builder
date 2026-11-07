package app

import (
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/revision"
)

func updateAtomicPrepared(t *testing.T, historyFailure bool, weight *string) *Operation {
	t.Helper()
	directory := t.TempDir()
	before := updateFixtureResult("До", nil, []*string{nil}, [][]string{{"Материал"}})
	after := updateFixtureResult("До", nil, []*string{nil}, [][]string{{"Новый материал"}})
	after.Document.Metadata.Order = weight
	old := updateFixtureSource(t, filepath.Join(directory, "old.json"), before)
	old.Document.Version = "5"
	if historyFailure {
		old.Document.Pages[0].Nodes[0].ID += "-v06"
	}
	archive := filepath.Join(directory, "module.zip")
	loaded := updateFixtureArchive(t, archive, []*lesson.Lesson{old}, []*string{nil}, []string{"10"})
	source := filepath.Join(directory, "new.json")
	candidate := updateFixtureSource(t, source, after)
	input, issues := parsed.Load(source)
	updateFixtureOK(t, issues)
	converted, issues := module.NewFile(loaded.Files[0].FileName(), candidate, diagnostic.Context{Path: source})
	updateFixtureOK(t, issues)
	state, issues := catalog.Import(loaded.Manifest, diagnostic.Context{Path: archive})
	updateFixtureOK(t, issues)
	op, issues := NewUpdate(directory, archive, source)
	updateFixtureOK(t, issues)
	// Здесь задаются типизированные предусловия private compareUpdate, без Analyze.
	op.at, op.baseline, op.beforeCatalog = updateFixtureAt(), loaded, state
	op.placement, op.oldFile = catalog.Clone(state), loaded.Files[0]
	op.inputs, op.converted = []*parsed.Input{input}, []*module.File{converted}
	updateFixtureOK(t, op.compareUpdate())
	op.analyzed = true
	return op
}

func TestUpdateApplyOwnerFailureCannotPublishPartialRevision(t *testing.T) {
	op := updateAtomicPrepared(t, true, nil)
	beforeCurrent, beforeRevised := op.current, op.revised
	beforeCatalog := catalog.Clone(op.placement)
	beforePlan := op.Snapshot().Plan
	_, _, expected := revision.Apply(op.oldFile.Lesson(), op.newFile.Lesson(), op.plan, op.fields, op.at, op.oldFile.Context(), op.newFile.Context())
	file, changed, issues := op.applyRevision()
	updateFlowCode(t, issues, revision.CodeHistory)
	updateFlowEqual(t, issues, expected)
	if file != nil || changed {
		t.Fatal("failed owner Apply returned partial file")
	}
	issues = op.PrepareRevision(op.Snapshot().Token)
	updateFlowCode(t, issues, revision.CodeHistory)
	updateFlowEqual(t, issues, expected)
	if op.current != beforeCurrent || op.revised != beforeRevised || op.revisionReady || op.lessonChanged {
		t.Fatal("owner Apply failure published partial state")
	}
	updateFlowEqual(t, op.placement, beforeCatalog)
	updateFlowEqual(t, op.Snapshot().Plan, beforePlan)
}

func TestUpdateSyncOwnerNilCandidateIsAtomic(t *testing.T) {
	op := updateAtomicPrepared(t, false, nil)
	// Отдельный корректный catalog с несовпадающим ID проверяет ветвь SyncLesson.
	input := updateFixtureResult("Другой", nil, []*string{nil}, [][]string{{"Другой материал"}})
	input.Document.Metadata.DocumentID = updateFixtureText("Other8")
	other := updateFixtureSource(t, filepath.Join(t.TempDir(), "other.json"), input)
	otherFile, issues := module.NewFile(op.oldFile.FileName(), other, op.oldFile.Context())
	updateFixtureOK(t, issues)
	state, issues := catalog.Create("Курс", "", []catalog.Item{{Entry: catalog.EntryFromLesson(other.Document, otherFile.FileName(), updateFixtureText("10"))}})
	updateFixtureOK(t, issues)
	op.placement = state
	beforeCurrent, beforeRevised, beforeKey := op.current, op.revised, op.key
	beforeCatalog := catalog.Clone(state)
	file, changed, issues := op.applyRevision()
	updateFixtureOK(t, issues)
	if !changed || file == nil {
		t.Fatal("fixture did not prepare successful owner revision")
	}
	candidate, issues := op.syncRevision(file)
	updateFlowCode(t, issues, catalog.CodeIdentity)
	_, expected := catalog.SyncLesson(catalog.Clone(state), file.FileName(), file.Lesson().Document)
	updateFlowEqual(t, issues, expected)
	if candidate != nil {
		t.Fatal("owner nil Sync candidate was replaced with fallback catalog")
	}
	issues = op.PrepareRevision(op.Snapshot().Token)
	updateFlowCode(t, issues, catalog.CodeIdentity)
	updateFlowEqual(t, issues, expected)
	if op.current != beforeCurrent || op.revised != beforeRevised || op.key != beforeKey || op.revisionReady || op.lessonChanged {
		t.Fatal("Sync failure published independently prepared File")
	}
	updateFlowEqual(t, op.placement, beforeCatalog)
}

func TestUpdatePlacementNilOwnerCandidateRetainsPendingDecision(t *testing.T) {
	op := updateAtomicPrepared(t, false, updateFixtureText("30"))
	proposals := op.proposals
	if len(proposals) != 1 || proposals[0].Field != "order" {
		t.Fatal("fixture must have one exact owner weight proposal")
	}
	// Типизированная private-граница: корректный catalog не содержит селектор.
	input := updateFixtureResult("Другой", nil, []*string{nil}, [][]string{{"Другой материал"}})
	input.Document.Metadata.DocumentID = updateFixtureText("Other8")
	other := updateFixtureSource(t, filepath.Join(t.TempDir(), "other.json"), input)
	names, issues := module.AssignFileNames(op.baseline.Files, 1, diagnostic.Context{})
	updateFixtureOK(t, issues)
	state, issues := catalog.Create("Курс", "", []catalog.Item{{Entry: catalog.EntryFromLesson(other.Document, names[0], updateFixtureText("10"))}})
	updateFixtureOK(t, issues)
	op.placement = state
	beforeKey, beforeCatalog := op.key, catalog.Clone(state)
	issues = op.DecidePlacement(op.Snapshot().Token, 0, true)
	updateFlowCode(t, issues, catalog.CodeSelection)
	if op.placementDecisions[0] != "" || op.key != beforeKey || op.Snapshot().PlacementAnswered[0] {
		t.Fatal("nil candidate consumed pending proposal or changed review identity")
	}
	updateFlowEqual(t, op.placement, beforeCatalog)
}
