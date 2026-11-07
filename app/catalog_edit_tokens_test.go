package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestCatalogEditActiveProposalBlocksEveryNewProposalAndWriting(t *testing.T) {
	op := catalogEditCreate(t, []string{"", ""})
	dir := t.TempDir()
	catalogEditOK(t, op.SetDestination(op.Snapshot().Token, dir, "result.zip"))
	before := op.Snapshot()
	saving, issues := op.ReviewSave(before.Token)
	catalogEditOK(t, issues)
	catalogEditOK(t, op.ConfirmSave(saving.Token, true, false))
	preview, issues := op.ProposeCourseTitle(before.Token, "Предложенный курс")
	catalogEditOK(t, issues)
	if preview == nil {
		t.Fatal("missing active proposal")
	}
	for _, test := range []struct {
		name string
		run  func() (*EditPreview, []*diagnostic.Error)
	}{
		{"course", func() (*EditPreview, []*diagnostic.Error) { return op.ProposeCourseTitle(before.Token, "Другой") }},
		{"unit", func() (*EditPreview, []*diagnostic.Error) { return op.ProposeUnitTitle(before.Token, "") }},
		{"section", func() (*EditPreview, []*diagnostic.Error) { return op.ProposeSection(before.Token, "Alpha") }},
		{"move", func() (*EditPreview, []*diagnostic.Error) { return op.ProposeMove(before.Token, "l001.json", nil) }},
		{"flat", func() (*EditPreview, []*diagnostic.Error) { return op.ProposeFlatPlacement(before.Token) }},
		{"weight", func() (*EditPreview, []*diagnostic.Error) {
			return op.ProposeLessonWeight(before.Token, "l001.json", "4")
		}},
		{"group", func() (*EditPreview, []*diagnostic.Error) {
			return op.ProposeLessonWeights(before.Token, nil, map[string]string{"l001.json": "4", "l002.json": "5"})
		}},
		{"sections", func() (*EditPreview, []*diagnostic.Error) { return op.ProposeSectionWeights(before.Token, nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			other, issues := test.run()
			if other != nil || len(issues) == 0 {
				t.Fatal("new proposal replaced an active proposal")
			}
		})
	}
	if next, issues := op.ReviewSave(before.Token); next != nil || len(issues) == 0 {
		t.Fatal("ReviewSave allowed an active edit")
	}
	result, issues := op.Save(catalogEditTime())
	if result.Installed || len(issues) == 0 || op.Snapshot().Actions.Save {
		t.Fatal("active edit retained write authorization")
	}
	if _, err := os.Stat(filepath.Join(dir, "result.zip")); !os.IsNotExist(err) {
		t.Fatal("active proposal allowed filesystem writing")
	}
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	after := op.Snapshot()
	if after.Catalog.CourseTitle != "Предложенный курс" || after.Actions.Save || after.Token == before.Token {
		t.Fatal("active proposal was replaced or old consent survived acceptance")
	}
	if len(op.ConfirmSave(saving.Token, true, false)) == 0 {
		t.Fatal("old save preview became valid after acceptance")
	}
	if next, issues := op.ReviewSave(before.Token); next != nil || len(issues) == 0 {
		t.Fatal("old accepted token permitted another save review")
	}
	newSave, issues := op.ReviewSave(after.Token)
	catalogEditOK(t, issues)
	if newSave == nil {
		t.Fatal("new accepted state cannot be reviewed")
	}
}

func TestCatalogEditTokensAreProposalSpecificAndOwned(t *testing.T) {
	op := catalogEditCreate(t, []string{"", ""})
	foreign := catalogEditCreate(t, []string{"", ""})
	before := op.Snapshot()
	proposal, issues := op.ProposeCourseTitle(before.Token, "Новый курс")
	catalogEditOK(t, issues)
	foreignProposal, issues := foreign.ProposeCourseTitle(foreign.Snapshot().Token, "Чужой")
	catalogEditOK(t, issues)
	for _, token := range []ReviewToken{{}, before.Token, foreignProposal.Token} {
		catalogEditCode(t, op.AcceptEdit(token), CodeStaleReview)
		catalogEditCode(t, op.RejectEdit(token), CodeStaleReview)
	}
	copied := *op
	if len(copied.AcceptEdit(proposal.Token)) == 0 || len(copied.RejectEdit(proposal.Token)) == 0 {
		t.Fatal("copied Operation accepted original owner's proposal")
	}
	catalogEditOK(t, op.RejectEdit(proposal.Token))
	catalogEditCode(t, op.RejectEdit(proposal.Token), CodeStaleReview)
	newProposal, issues := op.ProposeCourseTitle(before.Token, "Другой курс")
	catalogEditOK(t, issues)
	catalogEditCode(t, op.AcceptEdit(proposal.Token), CodeStaleReview)
	catalogEditOK(t, op.AcceptEdit(newProposal.Token))
	if op.Snapshot().Catalog.CourseTitle != "Другой курс" {
		t.Fatal("stale token disturbed the current proposal")
	}
	_, issues = op.ProposeCourseTitle(ReviewToken{}, "zero")
	catalogEditCode(t, issues, CodeStaleReview)
	_, issues = op.ProposeCourseTitle(foreign.Snapshot().Token, "foreign")
	catalogEditCode(t, issues, CodeStaleReview)
}

func TestCatalogEditProposalChecksBaseIdentity(t *testing.T) {
	for _, accept := range []bool{false, true} {
		op := catalogEditCreate(t, []string{"", ""})
		before := op.Snapshot()
		proposal, issues := op.ProposeCourseTitle(before.Token, "От устаревшей базы")
		catalogEditOK(t, issues)
		op.key = &reviewKey{owner: op}
		if accept {
			issues = op.AcceptEdit(proposal.Token)
		} else {
			issues = op.RejectEdit(proposal.Token)
		}
		catalogEditCode(t, issues, CodeStaleReview)
		if catalogEditJSON(t, op.Snapshot().Catalog) != catalogEditJSON(t, before.Catalog) {
			t.Fatal("stale base mutated accepted catalog")
		}
	}
}

func TestCatalogEditStageDistinguishesNilAndBlockedCandidate(t *testing.T) {
	op := catalogEditCreate(t, []string{"", ""})
	before := op.Snapshot()
	nilCandidate, ownerIssues := catalog.SetLessonWeight(before.Catalog, "l001.json", "0")
	preview, issues := op.stageCatalogEdit(nilCandidate, ownerIssues)
	if preview != nil || len(issues) == 0 || op.Snapshot().Token != before.Token {
		t.Fatal("nil owner result created a proposal or invalidated the accepted token")
	}
	candidate, ownerIssues := catalog.SetLessonWeight(before.Catalog, "l001.json", "02")
	preview, issues = op.stageCatalogEdit(candidate, ownerIssues)
	catalogEditCode(t, issues, catalog.CodeWeightConflict)
	if preview == nil || catalogEditJSON(t, op.Snapshot().Catalog) != catalogEditJSON(t, before.Catalog) {
		t.Fatal("blocked nonnil result was discarded or immediately published")
	}
	*catalogEditItem(t, preview.Catalog, "l001.json").Entry.Order = "99"
	op.AcceptEdit(preview.Token)
	if *catalogEditItem(t, op.Snapshot().Catalog, "l001.json").Entry.Order != "02" {
		t.Fatal("accept used exposed candidate")
	}
}

func TestCatalogEditRejectKeepsAcceptedTokenButDoesNotRestoreSaveConsent(t *testing.T) {
	op := catalogEditCreate(t, []string{"", ""})
	catalogEditOK(t, op.SetDestination(op.Snapshot().Token, t.TempDir(), "candidate.zip"))
	before := op.Snapshot()
	saving, issues := op.ReviewSave(before.Token)
	catalogEditOK(t, issues)
	catalogEditOK(t, op.ConfirmSave(saving.Token, true, false))
	preview, issues := op.ProposeCourseTitle(before.Token, "Отвергнутый курс")
	catalogEditOK(t, issues)
	catalogEditOK(t, op.RejectEdit(preview.Token))
	after := op.Snapshot()
	if after.Token != before.Token || after.Actions.Save || catalogEditJSON(t, after.Catalog) != catalogEditJSON(t, before.Catalog) {
		t.Fatal("reject changed accepted state or restored obsolete save consent")
	}
	if len(op.ConfirmSave(saving.Token, true, false)) == 0 {
		t.Fatal("proposal rejection restored old final consent")
	}
}
