package console

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func startTestTime(t *testing.T, text string, before, after time.Time) {
	t.Helper()
	got, err := time.Parse("2006-01-02T15:04:05Z", text)
	if err != nil || got.Before(before.UTC().Truncate(time.Second)) || got.After(after.UTC().Truncate(time.Second)) {
		t.Fatalf("time.Now не передан владельцу: %q (%v), window %s..%s", text, err, before, after)
	}
}

func TestStartAnalyzeUsesCurrentTimeAndLeavesHealthyOperation(t *testing.T) {
	base := t.TempDir()
	source := startTestSource(t, base, "json/урок.json", "BA82", nil)
	startTestUnchanged(t, source)
	op, issues := app.NewCreate(base, "json", "Курс", "")
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	var output bytes.Buffer
	d := dialog{operation: op, output: &output}
	before := time.Now()
	if err := d.analyze(); err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	snapshot := op.Snapshot()
	if snapshot.Module == nil || len(snapshot.Blockers) != 0 || !snapshot.Actions.EditCatalog || !snapshot.Actions.ChooseDestination {
		t.Fatalf("анализ отменён/не завершён: %+v output=%s", snapshot, &output)
	}
	startTestTime(t, snapshot.Module.Files[0].Lesson().Document.CreatedAt, before, after)
	startTestTime(t, snapshot.Module.Manifest.CreatedAt, before, after)
	if snapshot.Actions.Retry {
		t.Fatal("здоровый owner неожиданно разрешил Retry")
	}
	// Недоступность Retry здорового app не является ошибкой console.retry.
}

func TestStartAnalyzeShowsEveryRealSourceErrorAndNoFictionalSuccess(t *testing.T) {
	base := t.TempDir()
	var paths []string
	for _, name := range []string{"a.json", "b.JSON"} {
		path := filepath.Join(base, name)
		if err := os.WriteFile(path, []byte("{не JSON"), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	startTestUnchanged(t, paths...)
	op, issues := app.NewCreate(base, ".", "Курс", "")
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	var output bytes.Buffer
	d := dialog{operation: op, output: &output}
	if err := d.analyze(); err != nil {
		t.Fatalf("показанная предметная ошибка стала fatal: %v", err)
	}
	snapshot := op.Snapshot()
	if len(snapshot.Blockers) < 2 || !snapshot.Actions.Retry || snapshot.Actions.Save || snapshot.Module != nil {
		t.Fatalf("провал анализа: %+v", snapshot)
	}
	for _, issue := range snapshot.Blockers {
		for _, text := range []string{issue.Code, issue.Reason, issue.Context.Path} {
			if !strings.Contains(output.String(), text) {
				t.Fatalf("причина потеряна %q: %s", text, &output)
			}
		}
	}
	for _, text := range []string{"Проверка входных данных завершена", "Уроки преобразованы:", "Найдено JSON-файлов:"} {
		if strings.Contains(output.String(), text) {
			t.Fatalf("вымышленный промежуточный успех: %s", &output)
		}
	}
}

func TestStartAnalyzeKeepsCatalogEditableAndRetryRebuildsAllInputs(t *testing.T) {
	base := t.TempDir()
	resources := filepath.Join(base, "resources")
	if err := os.Mkdir(resources, 0700); err != nil {
		t.Fatal(err)
	}
	oldResource := filepath.Join(resources, "старый.txt")
	if err := os.WriteFile(oldResource, []byte("старые байты"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ name, id string }{{"a.json", "BA82"}, {"b.json", "NEW2"}} {
		startTestSource(t, base, filepath.Join("json", item.name), item.id, func(value *model.Result) {
			weight := "01"
			value.Document.Metadata.Order = &weight
			value.Document.Metadata.ResourceDirs = []string{"../resources"}
		})
	}
	op, issues := app.NewCreate(base, "json", "Исходный курс", "")
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	var output bytes.Buffer
	d := dialog{operation: op, output: &output}
	if err := d.analyze(); err != nil {
		t.Fatal(err)
	}
	blocked := op.Snapshot()
	if blocked.Catalog == nil || blocked.Module == nil || !blocked.Actions.EditCatalog || !blocked.Actions.Retry || len(blocked.Blockers) == 0 || len(blocked.Conflicts) == 0 {
		t.Fatalf("реальный конфликт не оставил редактируемый каталог: %+v", blocked)
	}
	edit, issues := op.ProposeCourseTitle(blocked.Token, "Временная правка")
	if edit == nil {
		t.Fatalf("каталог недоступен: %v", issues)
	}
	op.AcceptEdit(edit.Token) // Конфликт весов пока остаётся исправляемой блокировкой.
	if issues := op.SetDestination(op.Snapshot().Token, base, "отброшенная цель"); len(issues) != 0 {
		t.Fatal(issues)
	}
	oldToken := op.Snapshot().Token
	if err := os.Remove(oldResource); err != nil {
		t.Fatal(err)
	}
	newResource := filepath.Join(resources, "новый.txt")
	if err := os.WriteFile(newResource, []byte("новые байты\x00\xff"), 0600); err != nil {
		t.Fatal(err)
	}
	startTestSource(t, base, "json/b.json", "NEW2", func(value *model.Result) {
		weight, title := "2", "Исправленный исходный урок"
		value.Document.Metadata.Order, value.Document.Metadata.Title = &weight, &title
		value.Document.Metadata.ResourceDirs = []string{"../resources"}
	})
	startTestUnchanged(t, filepath.Join(base, "json/a.json"), filepath.Join(base, "json/b.json"), newResource)
	before := time.Now()
	if err := d.retry(); err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	fresh := op.Snapshot()
	if len(fresh.Blockers) != 0 || fresh.Target != nil || fresh.WriteResult != nil || fresh.Actions.Save || fresh.Catalog.CourseTitle != "Исходный курс" || len(fresh.Conflicts) != 0 {
		t.Fatalf("Retry сохранил старый план/цель: %+v", fresh)
	}
	if fresh.Resources.Total != 1 || fresh.Resources.Unused != 1 || len(fresh.Resources.UnusedPaths) != 1 || fresh.Resources.UnusedPaths[0] != newResource {
		t.Fatalf("Retry не обновил реальные resources: %+v", fresh.Resources)
	}
	found := false
	for _, item := range fresh.Catalog.Items {
		found = found || item.Entry.Title == "Исправленный исходный урок"
	}
	if !found {
		t.Fatal("Retry не перечитал JSON")
	}
	startTestTime(t, fresh.Module.Files[0].Lesson().Document.CreatedAt, before, after)
	if _, issues := op.ProposeCourseTitle(oldToken, "Старый token"); len(issues) == 0 {
		t.Fatal("Retry сохранил прежнее разрешение")
	}
}

func TestStartRetryAfterGenuineSourceErrorReadsRepairedFile(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "broken.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	op, issues := app.NewCreate(base, ".", "Курс", "")
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	d := dialog{operation: op, output: io.Discard}
	if err := d.analyze(); err != nil || !op.Snapshot().Actions.Retry {
		t.Fatalf("не получена реальная ошибка: %v", err)
	}
	startTestSource(t, base, "broken.json", "BA82", nil)
	startTestUnchanged(t, path)
	if err := d.retry(); err != nil {
		t.Fatal(err)
	}
	if snapshot := op.Snapshot(); len(snapshot.Blockers) != 0 || snapshot.Module == nil || !snapshot.Actions.EditCatalog {
		t.Fatalf("Retry не перечитал исправленный источник: %+v", snapshot)
	}
}

func TestStartAnalyzeAndRetryPropagateDiagnosticOutputFailure(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(map[bool]string{false: "analyze", true: "retry"}[retry], func(t *testing.T) {
			base := t.TempDir()
			op, issues := app.NewCreate(base, "missing", "Курс", "")
			if len(issues) != 0 {
				t.Fatal(issues)
			}
			if retry {
				op.Analyze(time.Now())
			}
			fault := errors.New("сломанный output")
			d := dialog{operation: op, output: startTestWriteFunc(func([]byte) (int, error) { return 0, fault })}
			run := d.analyze
			if retry {
				run = d.retry
			}
			err := run()
			var issue *diagnostic.Error
			if !errors.Is(err, fault) || !errors.As(err, &issue) {
				t.Fatalf("ошибка потока потеряна: %v", err)
			}
		})
	}
}
