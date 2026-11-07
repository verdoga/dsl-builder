package app

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
)

func TestSnapshotNilHasOnlyNavigationAndStateDiagnostic(t *testing.T) {
	var op *Operation
	s := op.Snapshot()
	if s.Actions != (Actions{MainMenu: true, Exit: true}) {
		t.Fatalf("nil actions: %+v", s.Actions)
	}
	if len(s.Blockers) != 1 {
		t.Fatalf("nil blockers: %+v", s.Blockers)
	}
	snapshotFixtureCode(t, s.Blockers[0], CodeState)
	if s.Token.key != nil || s.Module != nil || s.Catalog != nil || s.Target != nil ||
		s.WriteResult != nil || s.ManifestReady || s.NoChanges || s.Resources.Total != 0 {
		t.Fatalf("nil operation exposes prepared state: %+v", s)
	}
	s.Blockers[0].Reason = "изменено"
	if op.Snapshot().Blockers[0].Reason == "изменено" {
		t.Fatal("nil snapshots share diagnostics")
	}
}

func TestSnapshotReviewBelongsToExactLiveOperation(t *testing.T) {
	op := snapshotFixtureOperation(t, Add, false, false)
	current := op.Snapshot().Token
	if issue := op.checkReview(current); issue != nil {
		t.Fatal(issue)
	}
	foreign := snapshotFixtureOperation(t, Add, false, false)
	zero := &Operation{}
	var nilOperation *Operation
	for _, tc := range []struct {
		name  string
		op    *Operation
		token ReviewToken
		code  string
	}{
		{"zero token", op, ReviewToken{}, CodeStaleReview},
		{"foreign token", op, foreign.Snapshot().Token, CodeStaleReview},
		{"zero operation", zero, current, CodeState},
		{"nil operation", nilOperation, current, CodeState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshotFixtureCode(t, tc.op.checkReview(tc.token), tc.code)
		})
	}
	t.Run("copied operation", func(t *testing.T) {
		preview, issues := op.ReviewSave(current)
		snapshotFixtureOK(t, issues)
		snapshotFixtureOK(t, op.ConfirmSave(preview.Token, true, true))
		before, authorization := op.Snapshot(), op.saving
		original, err := os.ReadFile(op.archivePath)
		if err != nil {
			t.Fatal(err)
		}
		copyOfOperation := *op
		if issue := copyOfOperation.checkReview(current); issue == nil {
			t.Fatal("copied receiver accepted original review token")
		}
		if proposal, issues := copyOfOperation.ProposeCourseTitle(current, "Чужая правка"); proposal != nil || len(issues) == 0 {
			t.Fatal("copied receiver accepted a catalog change")
		}
		if review, issues := copyOfOperation.ReviewSave(current); review != nil || len(issues) == 0 {
			t.Fatal("copied receiver authorized a save review")
		}
		if len(copyOfOperation.ConfirmSave(preview.Token, true, true)) == 0 {
			t.Fatal("copied receiver accepted original save token")
		}
		if result, issues := copyOfOperation.Save(snapshotFixtureTime()); result.Installed || len(issues) == 0 {
			t.Fatal("copied receiver wrote using original authorization")
		}
		after, err := os.ReadFile(op.archivePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, after) || !reflect.DeepEqual(before, op.Snapshot()) || op.saving != authorization {
			t.Fatal("copied receiver changed original archive, model, token, or authorization")
		}
		if issue := op.checkReview(current); issue != nil || !op.Snapshot().Actions.Save {
			t.Fatalf("original owner lost its valid authorization: %v", issue)
		}
	})
	op.invalidateReview()
	snapshotFixtureCode(t, op.checkReview(current), CodeStaleReview)
	if issue := op.checkReview(op.Snapshot().Token); issue != nil {
		t.Fatalf("replacement token rejected: %v", issue)
	}
	op.finished = true
	snapshotFixtureCode(t, op.checkReview(op.Snapshot().Token), CodeState)
}

func TestSnapshotAcceptedEditRevokesAuthorizationButRejectedInputDoesNot(t *testing.T) {
	op := snapshotFixtureOperation(t, Create, false, false)
	token := op.Snapshot().Token
	original := op.placement.CourseTitle
	if proposal, issues := op.ProposeCourseTitle(token, "  "); proposal != nil || len(issues) == 0 {
		t.Fatalf("invalid title accepted: %+v %+v", proposal, issues)
	}
	if op.Snapshot().Token != token || op.placement.CourseTitle != original {
		t.Fatal("invalid input changed accepted state")
	}
	proposal, issues := op.ProposeCourseTitle(token, "Другой курс")
	snapshotFixtureOK(t, issues)
	if proposal == nil {
		t.Fatal("missing proposal")
	}
	if op.Snapshot().Token != token || op.placement.CourseTitle != original {
		t.Fatal("proposal changed accepted state before decision")
	}
	snapshotFixtureOK(t, op.RejectEdit(proposal.Token))
	if op.Snapshot().Token != token || op.placement.CourseTitle != original {
		t.Fatal("rejection changed accepted state")
	}
	preview, issues := op.ReviewSave(token)
	snapshotFixtureOK(t, issues)
	if preview == nil {
		t.Fatal("missing save review")
	}
	snapshotFixtureOK(t, op.ConfirmSave(preview.Token, true, false))
	proposal, issues = op.ProposeCourseTitle(token, "Принятый курс")
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, op.AcceptEdit(proposal.Token))
	if op.Snapshot().Token == token || op.placement.CourseTitle != "Принятый курс" || op.edit != nil || op.saving != nil {
		t.Fatal("accepted edit failed to publish and revoke authorizations")
	}
	snapshotFixtureCode(t, op.checkReview(token), CodeStaleReview)
	if len(op.ConfirmSave(preview.Token, true, false)) == 0 {
		t.Fatal("old save review authorized changed catalog")
	}
}

func TestSnapshotInvalidationDropsOnlyPendingAuthorization(t *testing.T) {
	op := snapshotFixtureOperation(t, Add, false, false)
	old := op.key
	candidate := catalog.Clone(op.placement)
	op.edit = &catalogEdit{key: &reviewKey{owner: op}, base: old, candidate: candidate}
	op.saving = &saveReview{key: &reviewKey{owner: op}, base: old, confirmed: true, replaceExisting: true}
	current, accepted := op.current, op.placement
	op.invalidateReview()
	if op.key == nil || op.key == old || op.key.owner != op || op.edit != nil || op.saving != nil {
		t.Fatal("accepted state identity/authorizations not replaced")
	}
	if op.current != current || op.placement != accepted || op.finished || !op.analyzed {
		t.Fatal("invalidation changed accepted model or operation stage")
	}
}
