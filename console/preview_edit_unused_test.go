package console

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/preview"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestConsolePreviewEditShowsRealCandidateIncludingEmptySectionAndBlockers(t *testing.T) {
	op := previewTestUpdate(t)
	snapshot := op.Snapshot()
	edit, _ := op.ProposeSection(snapshot.Token, "  Новая   пустая  ")
	if edit == nil || edit.Catalog == nil || len(edit.Blockers) == 0 || len(edit.Catalog.Sections) != 1 {
		t.Fatal("owner did not supply unfinished empty-section candidate")
	}
	before, err := json.Marshal(op.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	d := previewTestDialog(&output, "1\n")
	if err := d.showEdit(snapshot, edit); err != nil {
		t.Fatalf("editable blockers must be shown, not returned as presentation failure: %v", err)
	}
	composition, err := preview.Composition(edit.Catalog, snapshot.Module, snapshot.NewFileNames)
	previewTestContains(t, output.String(), previewTestBlock(t, composition, err), "Новая пустая", "(пустая рабочая секция)", "Не распределены", "l001.json", "Курс preview", "Юнит preview")
	changes, err := preview.ManifestChanges(edit.Changes)
	previewTestContains(t, output.String(), previewTestBlock(t, changes, err))
	for _, issue := range edit.Blockers {
		previewTestContains(t, output.String(), issue.Code, issue.Reason)
	}
	after, err := json.Marshal(op.Snapshot())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("showEdit mutated operation: %v", err)
	}
	if answer, err := d.readLine(); err != nil || answer != "1" {
		t.Fatalf("showEdit consumed an approval answer: %q, %v", answer, err)
	}
	// Реальное предложение по-прежнему открыто: showEdit его не принимает.
	previewTestIssues(t, op.RejectEdit(edit.Token))
	if op.Snapshot().Token != snapshot.Token {
		t.Fatal("showEdit accepted candidate or changed accepted token")
	}
}

func TestConsolePreviewUnusedRequiresNumericReturnAndDoesNotMutateOperation(t *testing.T) {
	op, _ := previewTestCreate(t, 1)
	snapshot := op.Snapshot()
	var registry resourceusage.Registry
	longPath := "/полный путь/" + string(bytes.Repeat([]byte("длинный-каталог/"), 30)) + "неиспользованный.wav"
	registry.Register([]string{longPath, "/полный путь/первый.txt", "/полный путь/used.mp3"})
	registry.MarkUsed([]string{"/полный путь/used.mp3"})
	snapshot.Resources = registry.Snapshot()
	before, err := json.Marshal(op.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	d := previewTestDialog(&output, "да\n2\n\n1\nоставлено\n")
	d.operation = op
	if err := d.showUnused(snapshot); err != nil {
		t.Fatal(err)
	}
	previewTestContains(t, output.String(), preview.UnusedResources(snapshot.Resources), longPath, "/полный путь/первый.txt", "Всего файлов: 3", "Использовано: 1", "Не использовано: 2", "ресурсы обрабатываемых исходников текущей операции", "1. Вернуться к предварительному просмотру")
	next, err := d.readLine()
	if err != nil || next != "оставлено" {
		t.Fatalf("return consumed wrong answer: %q, %v", next, err)
	}
	after, err := json.Marshal(op.Snapshot())
	if err != nil || !bytes.Equal(before, after) || op.Snapshot().Token != snapshot.Token {
		t.Fatalf("return changed operation: %v", err)
	}
}

func TestConsolePreviewUnusedEmptyReportStillReturnsByMenu(t *testing.T) {
	var output bytes.Buffer
	if err := previewTestDialog(&output, "1\n").showUnused(app.Snapshot{}); err != nil {
		t.Fatal(err)
	}
	previewTestContains(t, output.String(), "Неиспользованных файлов нет", "Всего файлов: 0", "1. Вернуться к предварительному просмотру")
}

func TestConsolePreviewNeverPrintsOwnNodeDataAnswersHintsOrBase64(t *testing.T) {
	snapshot := previewTestReadyUpdate(t).Snapshot()
	value := snapshot.After.Lesson()
	value.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value = "SECRET-NODE-CONTENT"
	value.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Answers = []string{"SECRET-ANSWER"}
	value.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Hint = "SECRET-HINT"
	encoded := base64.StdEncoding.EncodeToString([]byte{0, 255, 72, 111, 13, 10, 0, 99})
	role := lesson.RoleBody
	value.Document.Pages[0].Nodes = append(value.Document.Pages[0].Nodes, lesson.Node{
		ID: "ba82-n999-v02", Type: lesson.NodeMedia, Data: []lesson.Data{{Type: lesson.DataMedia, Role: &role, Value: lesson.EmbeddedFile{FileName: "SECRET-FILENAME.png", MIMEType: "image/png", Content: encoded}, Answers: []string{}, Hint: ""}}, Nodes: []lesson.Node{},
	})
	file, issues := module.NewFile("l001.json", value, snapshot.After.Context())
	previewTestIssues(t, issues)
	// Валидированный owner File в typed проекции: не сценарий изменения app.
	snapshot.After = file
	snapshot.Module.Files = []*module.File{file}
	var output bytes.Buffer
	if err := previewTestDialog(&output, "").showSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	previewTestContains(t, output.String(), "ba82-n002-v02", "l001.json", "FRESH")
	previewTestAbsent(t, output.String(), "SECRET-NODE-CONTENT", "SECRET-ANSWER", "SECRET-HINT", "SECRET-FILENAME.png", encoded)
}
