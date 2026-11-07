package app

import (
	"bytes"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestCatalogEditUpdateRequiresEveryInitialPlacementDecision(t *testing.T) {
	archive := catalogEditArchive(t, []string{"", ""})
	dir := t.TempDir()
	source := catalogEditSource(t, dir, "update.json", "CE00", "Урок 0", catalogEditString("Alpha"), catalogEditString("7"))
	op, issues := NewUpdate(dir, archive, source)
	catalogEditOK(t, issues)
	catalogEditOK(t, op.Analyze(catalogEditTime()))
	before := op.Snapshot()
	if len(before.PlacementProposals) != 2 {
		t.Fatalf("fixture must propose section and order: %+v", before.PlacementProposals)
	}
	catalogEditOK(t, op.PrepareRevision(before.Token))
	for decided := 0; decided < 2; decided++ {
		token := op.Snapshot().Token
		for _, run := range []func() (*EditPreview, []*diagnostic.Error){
			func() (*EditPreview, []*diagnostic.Error) {
				return op.ProposeCourseTitle(token, "Ручной курс")
			},
			func() (*EditPreview, []*diagnostic.Error) { return op.ProposeUnitTitle(token, "Ручной юнит") },
			func() (*EditPreview, []*diagnostic.Error) { return op.ProposeSection(token, "Alpha") },
			func() (*EditPreview, []*diagnostic.Error) { return op.ProposeMove(token, "l001.json", nil) },
			func() (*EditPreview, []*diagnostic.Error) { return op.ProposeFlatPlacement(token) },
			func() (*EditPreview, []*diagnostic.Error) { return op.ProposeLessonWeight(token, "l001.json", "4") },
			func() (*EditPreview, []*diagnostic.Error) {
				return op.ProposeLessonWeights(token, nil, map[string]string{"l001.json": "4", "l002.json": "5"})
			},
			func() (*EditPreview, []*diagnostic.Error) {
				return op.ProposeSectionWeights(token, []catalog.SectionOrder{})
			},
		} {
			if preview, issues := run(); preview != nil || len(issues) == 0 {
				t.Fatal("manual edit bypassed unresolved initial placement")
			}
		}
		catalogEditOK(t, op.DecidePlacement(op.Snapshot().Token, decided, false))
	}
	afterDecisions := op.Snapshot()
	if catalogEditJSON(t, afterDecisions.Catalog) != catalogEditJSON(t, before.Catalog) {
		t.Fatal("rejected placement proposals changed accepted placement")
	}
	preview, issues := op.ProposeCourseTitle(afterDecisions.Token, "Ручной курс")
	catalogEditOK(t, issues)
	if preview == nil {
		t.Fatal("all initial placement decisions did not unlock manual edits")
	}
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	if op.Snapshot().Actions.Save {
		t.Fatal("manual catalog consent became save consent")
	}
}

func TestCatalogEditUpdateFieldConsentDoesNotAuthorizeArchiveWriting(t *testing.T) {
	archive := catalogEditArchive(t, []string{"", ""})
	beforeBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := catalogEditSource(t, dir, "update.json", "CE00", "Новое название", nil, catalogEditString("1"))
	op, issues := NewUpdate(dir, archive, source)
	catalogEditOK(t, issues)
	catalogEditOK(t, op.Analyze(catalogEditTime()))
	snapshot := op.Snapshot()
	if len(snapshot.Plan.Fields) != 1 || len(snapshot.PlacementProposals) != 0 {
		t.Fatal("fixture must have one separate field decision")
	}
	catalogEditOK(t, op.DecideField(snapshot.Token, 0, true))
	catalogEditOK(t, op.PrepareRevision(op.Snapshot().Token))
	preview, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Название курса")
	catalogEditOK(t, issues)
	accepted := op.Snapshot()
	catalogEditOK(t, op.RejectEdit(preview.Token))
	rejected := op.Snapshot()
	if rejected.Token != accepted.Token || !rejected.FieldAnswered[0] || !rejected.FieldAccepted[0] || !rejected.RevisionReady || catalogEditComposition(t, rejected) != catalogEditComposition(t, accepted) {
		t.Fatal("rejecting a catalog edit invalidated accepted update decisions or files")
	}
	preview, issues = op.ProposeCourseTitle(rejected.Token, "Название курса")
	catalogEditOK(t, issues)
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	snapshot = op.Snapshot()
	if !snapshot.FieldAnswered[0] || !snapshot.FieldAccepted[0] || snapshot.After.Lesson().Document.Title != "Новое название" {
		t.Fatal("catalog edit lost independently accepted field decision")
	}
	if snapshot.Actions.Save {
		t.Fatal("individual field or catalog consent authorized writing")
	}
	result, issues := op.Save(catalogEditTime())
	if result.Installed || len(issues) == 0 {
		t.Fatal("archive written without final consent")
	}
	afterBytes, err := os.ReadFile(archive)
	if err != nil || !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatal("archive changed before final save consent")
	}
}

func TestCatalogEditNilReceiverReturnsStateDiagnostics(t *testing.T) {
	var op *Operation
	for _, run := range []func() (*EditPreview, []*diagnostic.Error){
		func() (*EditPreview, []*diagnostic.Error) { return op.ProposeCourseTitle(ReviewToken{}, "Курс") },
		func() (*EditPreview, []*diagnostic.Error) { return op.ProposeUnitTitle(ReviewToken{}, "Юнит") },
		func() (*EditPreview, []*diagnostic.Error) { return op.ProposeSection(ReviewToken{}, "Alpha") },
		func() (*EditPreview, []*diagnostic.Error) { return op.ProposeMove(ReviewToken{}, "l001.json", nil) },
		func() (*EditPreview, []*diagnostic.Error) { return op.ProposeFlatPlacement(ReviewToken{}) },
		func() (*EditPreview, []*diagnostic.Error) {
			return op.ProposeLessonWeight(ReviewToken{}, "l001.json", "1")
		},
		func() (*EditPreview, []*diagnostic.Error) { return op.ProposeLessonWeights(ReviewToken{}, nil, nil) },
		func() (*EditPreview, []*diagnostic.Error) { return op.ProposeSectionWeights(ReviewToken{}, nil) },
	} {
		preview, issues := run()
		catalogEditCode(t, issues, CodeState)
		if preview != nil {
			t.Fatal("nil receiver produced an edit preview")
		}
	}
	catalogEditCode(t, op.AcceptEdit(ReviewToken{}), CodeState)
	catalogEditCode(t, op.RejectEdit(ReviewToken{}), CodeState)
}
