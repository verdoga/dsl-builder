package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/preview"
)

func TestConsoleIntegrationEveryFieldSeparatelyIncludingUnknownNullPage(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject", true: "accept"}[accept], func(t *testing.T) {
			base := t.TempDir()
			old := filepath.Join(base, "old")
			integrationTestStructuredSource(t, old, "FIELDS41", "Old title", integrationTestPointer("Old subtitle"), [][]string{{"same"}}, []*string{integrationTestPointer("Old page")})
			archive := integrationTestSeed(t, base, old, "fields.zip")
			source := integrationTestUnknownNullSource(t, filepath.Join(base, "new"))
			archiveBytes, sourceBytes := integrationTestBytes(t, archive), integrationTestBytes(t, source)
			op, issues := app.NewUpdate(base, archive, source)
			integrationTestIssues(t, issues)
			integrationTestIssues(t, op.Analyze(time.Now()))
			initial := op.Snapshot()
			if initial.Plan == nil {
				t.Fatal("missing plan")
			}
			expectedFields := []compare.FieldChange{
				{Field: compare.DocumentTitle, Before: integrationTestPointer("Old title"), After: integrationTestPointer("New title"), OldPage: -1, NewPage: -1},
				{Field: compare.DocumentSubtitle, Before: integrationTestPointer("Old subtitle"), OldPage: -1, NewPage: -1},
				{Field: compare.PageTitle, Before: integrationTestPointer("Old page"), OldPage: 0, NewPage: 0},
				{Field: compare.PageTitle, After: integrationTestPointer("New page"), OldPage: -1, NewPage: 1},
				{Field: compare.PageTitle, OldPage: -1, NewPage: 2},
			}
			if !reflect.DeepEqual(initial.Plan.Fields, expectedFields) {
				t.Fatalf("raw fixture did not produce the exact known/null and unknown/null field plan: %+v", initial.Plan.Fields)
			}
			seen := map[compare.Field]bool{}
			unknownNull := false
			knownNull := false
			for index, change := range initial.Plan.Fields {
				seen[change.Field] = true
				unknownNull = unknownNull || (change.Field == compare.PageTitle && change.OldPage == -1 && change.After == nil)
				knownNull = knownNull || (change.Field == compare.PageTitle && change.OldPage == 0 && change.Before != nil && *change.Before == "Old page" && change.After == nil)
				snapshot := op.Snapshot()
				expected, err := preview.FieldProposal(snapshot.Before, change)
				if err != nil {
					t.Fatal(err)
				}
				driver := &integrationTestDriver{t: t, steps: []integrationTestStep{{literal: map[bool]string{false: "2", true: "1"}[accept], check: func(s string) {
					integrationTestContains(t, s, expected)
					if change.Field == compare.PageTitle && change.OldPage == -1 && change.After == nil {
						integrationTestContains(t, s, "Было: прежнее поле не установлено", "Станет: null", "Позиция новой страницы: 3", "При отказе новая страница останется без названия (title=null).")
					}
					if !bytes.Equal(archiveBytes, integrationTestBytes(t, archive)) || !bytes.Equal(sourceBytes, integrationTestBytes(t, source)) {
						t.Fatal("field review wrote archive or source before separate consent")
					}
				}}}}
				d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
				if err := d.decideField(snapshot); err != nil {
					t.Fatal(err)
				}
				next := op.Snapshot()
				if !next.FieldAnswered[index] || next.FieldAccepted[index] != accept {
					t.Fatalf("field %d decision lost", index)
				}
				if len(driver.steps) != 0 {
					t.Fatal("field did not request its separate confirmation")
				}
				for j := 0; j <= index; j++ {
					if !next.FieldAnswered[j] || next.FieldAccepted[j] != accept {
						t.Fatal("later field decision changed an earlier answer")
					}
				}
				for j := index + 1; j < len(next.FieldAnswered); j++ {
					if next.FieldAnswered[j] {
						t.Fatal("one answer accepted later field")
					}
				}
			}
			if !seen[compare.DocumentTitle] || !seen[compare.DocumentSubtitle] || !seen[compare.PageTitle] || !unknownNull || !knownNull {
				t.Fatalf("fixture omitted field/null cases: %+v", initial.Plan.Fields)
			}
			integrationTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
			revised := op.Snapshot().After.Lesson().Document
			wantTitle, wantSubtitle := "Old title", integrationTestPointer("Old subtitle")
			wantPages := []*string{integrationTestPointer("Old page"), nil, nil}
			if accept {
				wantTitle, wantSubtitle = "New title", nil
				wantPages = []*string{nil, integrationTestPointer("New page"), nil}
			}
			if revised.Title != wantTitle || !reflect.DeepEqual(revised.Subtitle, wantSubtitle) || len(revised.Pages) != len(wantPages) {
				t.Fatal("separate answers were not applied to document fields/pages")
			}
			for i, title := range wantPages {
				if !reflect.DeepEqual(revised.Pages[i].Title, title) {
					t.Fatalf("separate page answer %d was not applied", i)
				}
			}
			if !bytes.Equal(archiveBytes, integrationTestBytes(t, archive)) || !bytes.Equal(sourceBytes, integrationTestBytes(t, source)) {
				t.Fatal("field decisions or revision preparation wrote archive/source")
			}
		})
	}
}

// integrationTestUnknownNullSource builds the raw boundary before parsed.Load.
// The first page uniquely matches old content; the other two are unmatched.
// @newpage is required for a later page whose title is genuinely null.
func integrationTestUnknownNullSource(t *testing.T, dir string) string {
	t.Helper()
	value := model.Result{Format: model.FormatVersion1, Document: model.Document{DSLVersion: integrationTestPointer("1.2"), Encoding: integrationTestPointer("UTF-8"), HasBOM: integrationTestPointer(false), Metadata: model.DocumentMetadata{DocumentID: integrationTestPointer("FIELDS41"), Title: integrationTestPointer("New title"), Order: integrationTestPointer("1"), ResourceDirs: []string{}}}, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{}, Lines: []model.Line{}}
	element := func(kind model.ElementType, raw, content string, start int) model.Element {
		return model.Element{ElementType: kind, Raw: raw, Value: &content, Start: start, End: start + len([]rune(raw)), ErrorIDs: []string{}}
	}
	add := func(raw string, kind model.LineType, parent *int, elements ...model.Element) {
		nesting := 0
		if parent != nil {
			nesting = 1
		}
		value.Lines = append(value.Lines, model.Line{Number: len(value.Lines) + 1, LineType: kind, NestingLevel: nesting, ParentLine: parent, Raw: raw, LineEnding: model.LineEndingLF, Elements: elements})
	}
	task := func(body string) {
		add("@task a", model.LineTypeTag, nil, element(model.ElementTypeTag, "@task", "task", 1), element(model.ElementTypeIdentifier, "a", "a", 7))
		parent := len(value.Lines)
		add("@instr "+body, model.LineTypeTag, &parent, element(model.ElementTypeTag, "@instr", "instr", 1), element(model.ElementTypeContent, body, body, 8))
	}
	task("same")
	add("### New page", model.LineTypeHeading, nil, element(model.ElementTypeHeadingLevel, "###", "3", 1), element(model.ElementTypeTitle, "New page", "New page", 5))
	task("other")
	add("@newpage", model.LineTypeTag, nil, element(model.ElementTypeTag, "@newpage", "newpage", 1))
	task("unmatched")
	value.Document.LineCount = integrationTestPointer(len(value.Lines))
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "FIELDS41.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, issues := parsed.Load(path)
	integrationTestIssues(t, issues)
	return path
}

func TestConsoleIntegrationReplacementLevelsUseReadyOwnerPreview(t *testing.T) {
	for _, level := range []string{"node", "page", "lesson"} {
		t.Run(level, func(t *testing.T) {
			base := t.TempDir()
			old := filepath.Join(base, "old")
			title := integrationTestPointer("Page")
			integrationTestStructuredSource(t, old, "IDS41", "Title", nil, [][]string{{"secret old", "stable"}}, []*string{title})
			archive := integrationTestSeed(t, base, old, "ids.zip")
			pages := [][]string{{"secret new", "stable"}}
			titles := []*string{title}
			if level == "page" {
				pages = [][]string{{"secret old", "stable", "inserted"}}
			}
			if level == "lesson" {
				pages = [][]string{{"secret old", "stable"}, {"second"}}
				titles = append(titles, integrationTestPointer("Second"))
			}
			source := integrationTestStructuredSource(t, filepath.Join(base, "new"), "IDS41", "Title", nil, pages, titles)
			op, issues := app.NewUpdate(base, archive, source)
			integrationTestIssues(t, issues)
			integrationTestIssues(t, op.Analyze(time.Now()))
			for {
				s := op.Snapshot()
				pending := -1
				for i, answered := range s.FieldAnswered {
					if !answered {
						pending = i
						break
					}
				}
				if pending < 0 {
					break
				}
				integrationTestIssues(t, op.DecideField(s.Token, pending, true))
			}
			integrationTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
			s := op.Snapshot()
			expected, err := preview.Replacements(s.Before, s.After, s.Plan)
			if err != nil {
				t.Fatal(err)
			}
			driver := &integrationTestDriver{t: t}
			d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
			if err := d.showUpdate(s); err != nil {
				t.Fatal(err)
			}
			integrationTestContains(t, driver.output.String(), expected, "IDS41")
			for _, secret := range []string{"secret old", "secret new", "inserted", "c2VjcmV0"} {
				if strings.Contains(driver.output.String(), secret) {
					t.Fatalf("content/Base64 leaked: %s", secret)
				}
			}
		})
	}
}
