package console

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/preview"
)

func previewTestPresentation(t *testing.T, err error) {
	t.Helper()
	var issue *diagnostic.Error
	if err == nil || !errors.As(err, &issue) || issue.Code != CodePreview || issue.Cause == nil || isStreamFailure(err) {
		t.Fatalf("want CodePreview with cause and no stream failure, got %v", err)
	}
}

func TestConsolePreviewUpdateJoinsAllThreeIndependentErrors(t *testing.T) {
	snapshot := app.Snapshot{Mode: app.Update, RevisionReady: true}
	_, headingErr := preview.UpdateHeading("", nil, nil)
	_, fieldsErr := preview.AcceptedFields(nil, nil, nil)
	_, replacementsErr := preview.Replacements(nil, nil, nil)
	var output bytes.Buffer
	err := previewTestDialog(&output, "").showUpdate(snapshot)
	previewTestPresentation(t, err)
	previewTestPresentationCauses(t, err, headingErr, fieldsErr, replacementsErr)
}

func TestConsolePreviewUpdateContinuesAfterAcceptedFieldFailure(t *testing.T) {
	snapshot := previewTestReadyUpdate(t).Snapshot()
	snapshot.AcceptedFields = []compare.FieldChange{{Field: compare.PageTitle, OldPage: 0, NewPage: 999}}
	var output bytes.Buffer
	err := previewTestDialog(&output, "").showUpdate(snapshot)
	previewTestPresentation(t, err)
	heading, ownerErr := preview.UpdateHeading(snapshot.ArchivePath, snapshot.Before, snapshot.After)
	previewTestContains(t, output.String(), previewTestBlock(t, heading, ownerErr))
	replacements, ownerErr := preview.Replacements(snapshot.Before, snapshot.After, snapshot.Plan)
	previewTestContains(t, output.String(), previewTestBlock(t, replacements, ownerErr), "ba82-n002-v02")
}

func TestConsolePreviewSnapshotJoinsFailuresAndKeepsLaterBlocks(t *testing.T) {
	snapshot := previewTestReadyUpdate(t).Snapshot()
	snapshot.Module.Files = nil
	snapshot.Before, snapshot.After, snapshot.Plan = nil, nil, nil
	snapshot.ManifestChanges = []catalog.Change{{Field: "course.title", After: []byte(`{`)}}
	snapshot.Blockers = []*diagnostic.Error{{Code: "fixture.last", Reason: "Последняя блокировка"}}
	snapshot.Actions.UnusedResources = true
	_, compositionErr := preview.Composition(snapshot.Catalog, snapshot.Module, snapshot.NewFileNames)
	_, headingErr := preview.UpdateHeading(snapshot.ArchivePath, nil, nil)
	_, fieldsErr := preview.AcceptedFields(nil, nil, snapshot.AcceptedFields)
	_, replacementsErr := preview.Replacements(nil, nil, nil)
	_, manifestErr := preview.ManifestChanges(snapshot.ManifestChanges)
	var output bytes.Buffer
	err := previewTestDialog(&output, "").showSnapshot(snapshot)
	previewTestPresentation(t, err)
	previewTestPresentationCauses(t, err, compositionErr, headingErr, fieldsErr, replacementsErr, manifestErr)
	previewTestContains(t, output.String(), "fixture.last", "Последняя блокировка", preview.Resources(snapshot.Resources))
}

func TestConsolePreviewEditFailureInAnyBlockPreventsSuccessfulPresentation(t *testing.T) {
	snapshot := previewTestReadyUpdate(t).Snapshot()
	for _, test := range []struct {
		name string
		edit *app.EditPreview
	}{
		{"composition", &app.EditPreview{Catalog: nil, Changes: []catalog.Change{{Field: "course.title", After: []byte(`"Поздняя правка"`)}}, Blockers: []*diagnostic.Error{{Code: "fixture.edit", Reason: "Блокировка кандидата"}}}},
		{"both", &app.EditPreview{Catalog: nil, Changes: []catalog.Change{{Field: "course.title", After: []byte(`{`)}}, Blockers: []*diagnostic.Error{{Code: "fixture.edit", Reason: "Блокировка кандидата"}}}},
		{"manifest", &app.EditPreview{Catalog: snapshot.Catalog, Changes: []catalog.Change{{Field: "course.title", After: []byte(`{`)}}, Blockers: []*diagnostic.Error{{Code: "fixture.edit", Reason: "Блокировка кандидата"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := previewTestDialog(&output, "").showEdit(snapshot, test.edit)
			previewTestPresentation(t, err)
			previewTestContains(t, output.String(), "fixture.edit", "Блокировка кандидата")
			if test.name == "composition" {
				previewTestContains(t, output.String(), "Поздняя правка")
			} else if test.name == "manifest" {
				previewTestContains(t, output.String(), "Курс preview", "l001.json")
			}
		})
	}
}

// previewTestFaultWriter имитирует только сбой стандартной границы io.Writer.
type previewTestFaultWriter struct {
	remaining    int
	failToken    string
	cause        error
	failed       bool
	afterFailure int
	output       bytes.Buffer
}

func (w *previewTestFaultWriter) Write(p []byte) (int, error) {
	if w.failed {
		w.afterFailure++
		return 0, w.cause
	}
	if w.failToken != "" {
		if !strings.Contains(string(p), w.failToken) {
			return w.output.Write(p)
		}
		w.failed = true
		return 0, w.cause
	}
	if len(p) <= w.remaining {
		w.remaining -= len(p)
		return w.output.Write(p)
	}
	n := w.remaining
	w.output.Write(p[:n])
	w.remaining, w.failed = 0, true
	return n, w.cause
}

func TestConsolePreviewStreamFailureStopsEveryCoordinatorImmediately(t *testing.T) {
	snapshot := previewTestReadyUpdate(t).Snapshot()
	edit := &app.EditPreview{Catalog: snapshot.Catalog, Changes: snapshot.ManifestChanges}
	methods := []struct {
		name string
		call func(*dialog) error
	}{
		{"snapshot", func(d *dialog) error { return d.showSnapshot(snapshot) }},
		{"composition", func(d *dialog) error { return d.showComposition(snapshot) }},
		{"update", func(d *dialog) error { return d.showUpdate(snapshot) }},
		{"manifest", func(d *dialog) error { return d.showManifest(snapshot) }},
		{"edit", func(d *dialog) error { return d.showEdit(snapshot, edit) }},
		{"unused", func(d *dialog) error { return d.showUnused(snapshot) }},
	}
	for _, method := range methods {
		for _, short := range []bool{false, true} {
			t.Run(method.name+map[bool]string{false: "/cause", true: "/short"}[short], func(t *testing.T) {
				cause := errors.New("writer stopped")
				pathErr := &os.PathError{Op: "write", Path: "/stream", Err: cause}
				writer := &previewTestFaultWriter{remaining: 7, cause: pathErr}
				expected := error(cause)
				if short {
					writer.cause, expected = nil, io.ErrShortWrite
				}
				d := &dialog{output: writer}
				err := method.call(d)
				if !errors.Is(err, expected) || !isStreamFailure(err) || writer.afterFailure != 0 || !writer.failed {
					t.Fatalf("stream error/stop lost: err=%v, further writes=%d", err, writer.afterFailure)
				}
				var issue *diagnostic.Error
				if !errors.As(err, &issue) || issue.Code != CodeOutput {
					t.Fatalf("output error became presentation failure: %v", err)
				}
				if !short {
					var original *os.PathError
					if !errors.As(err, &original) || original != pathErr {
						t.Fatalf("original typed cause lost: %v", err)
					}
				}
			})
		}
	}
}

func TestConsolePreviewStreamFailurePreservesOriginalBlockersAndEarlierPresentationError(t *testing.T) {
	snapshot := previewTestReadyUpdate(t).Snapshot()
	snapshot.Module.Files = nil
	snapshot.Mode = app.Create
	snapshot.ManifestChanges = nil
	sentinel := errors.New("original blocker cause")
	blocker := &diagnostic.Error{Code: "fixture.blocked", Reason: "Блокировка", Cause: sentinel}
	snapshot.Blockers = []*diagnostic.Error{blocker}
	writerCause := errors.New("failed output")
	_, ownerErr := preview.Composition(snapshot.Catalog, snapshot.Module, snapshot.NewFileNames)
	t.Run("earlier-presentation", func(t *testing.T) {
		writer := &previewTestFaultWriter{cause: writerCause}
		err := (&dialog{output: writer}).showSnapshot(snapshot)
		if !errors.Is(err, writerCause) || !isStreamFailure(err) || !writer.failed || writer.afterFailure != 0 {
			t.Fatalf("lost stream failure: %v", err)
		}
		previewTestPresentationCauses(t, err, ownerErr)
	})
	// Отдельный subtest выполняется даже при ошибке предыдущего oracle.
	t.Run("original-blocker", func(t *testing.T) {
		writer := &previewTestFaultWriter{cause: writerCause, failToken: blocker.Code}
		err := (&dialog{output: writer}).showEdit(snapshot, &app.EditPreview{Catalog: nil, Blockers: []*diagnostic.Error{blocker}})
		if !errors.Is(err, sentinel) || !errors.Is(err, blocker) || !errors.Is(err, writerCause) || !isStreamFailure(err) || !writer.failed || writer.afterFailure != 0 {
			t.Fatalf("original blocker/stream causes not preserved: %v", err)
		}
		_, editErr := preview.Composition(nil, snapshot.Module, snapshot.NewFileNames)
		previewTestPresentationCauses(t, err, editErr)
	})
}
