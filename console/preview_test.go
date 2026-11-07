package console

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/preview"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestConsolePreviewFailedAnalysisDoesNotRequireReadyModels(t *testing.T) {
	for _, mode := range []app.Mode{app.Create, app.Update, app.Add} {
		t.Run(string(mode), func(t *testing.T) {
			base := t.TempDir()
			var op *app.Operation
			var issues []*diagnostic.Error
			switch mode {
			case app.Create:
				op, issues = app.NewCreate(base, "missing", "Курс", "")
			case app.Update:
				op, issues = app.NewUpdate(base, "missing.zip", "missing.json")
			case app.Add:
				op, issues = app.NewAdd(base, "missing.zip", "missing.json")
			}
			previewTestIssues(t, issues)
			t.Cleanup(op.Cancel)
			if len(op.Analyze(time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC))) == 0 {
				t.Fatal("fixture should fail analysis")
			}
			snapshot := op.Snapshot()
			if snapshot.Catalog != nil || snapshot.Module != nil || len(snapshot.Blockers) == 0 {
				t.Fatal("expected owner snapshot without analyzed models and with blockers")
			}
			var output bytes.Buffer
			if err := previewTestDialog(&output, "").showSnapshot(snapshot); err != nil {
				t.Fatalf("diagnosed analysis failure became a mandatory-preview error: %v", err)
			}
			for _, issue := range snapshot.Blockers {
				previewTestContains(t, output.String(), issue.Code, issue.Reason)
			}
			previewTestAbsent(t, output.String(), "Версия урока:", "Будет заменена нода:")
		})
	}
}

func TestConsolePreviewSnapshotKeepsAllPreparedBlocksAndResourceGate(t *testing.T) {
	op := previewTestReadyUpdate(t)
	snapshot := op.Snapshot()
	if snapshot.Plan.Fields[1].After == nil || *snapshot.Plan.Fields[1].After != "Prepared" || snapshot.After.Lesson().Document.Subtitle == nil || *snapshot.After.Lesson().Document.Subtitle != "Subtitle" {
		t.Fatal("fixture must propose Prepared and retain rejected subtitle Subtitle")
	}
	var registry resourceusage.Registry
	registry.Register([]string{"/источники/unused.txt", "/источники/used.mp3"})
	registry.MarkUsed([]string{"/источники/used.mp3"})
	snapshot.Resources = registry.Snapshot()
	snapshot.Blockers = []*diagnostic.Error{{Code: "fixture.one", Reason: "Первая блокировка"}, {Code: "fixture.two", Reason: "Вторая блокировка"}}
	composition, err := preview.Composition(snapshot.Catalog, snapshot.Module, snapshot.NewFileNames)
	composition = previewTestBlock(t, composition, err)
	heading, err := preview.UpdateHeading(snapshot.ArchivePath, snapshot.Before, snapshot.After)
	heading = previewTestBlock(t, heading, err)
	fields, err := preview.AcceptedFields(snapshot.Before, snapshot.After, snapshot.AcceptedFields)
	fields = previewTestBlock(t, fields, err)
	replacements, err := preview.Replacements(snapshot.Before, snapshot.After, snapshot.Plan)
	replacements = previewTestBlock(t, replacements, err)
	changes, err := preview.ManifestChanges(snapshot.ManifestChanges)
	changes = previewTestBlock(t, changes, err)
	for _, available := range []bool{false, true} {
		t.Run(map[bool]string{false: "without-report", true: "with-report"}[available], func(t *testing.T) {
			snapshot.Actions.UnusedResources = available
			var output bytes.Buffer
			if err := previewTestDialog(&output, "").showSnapshot(snapshot); err != nil {
				t.Fatal(err)
			}
			previewTestContains(t, output.String(), composition, heading, fields, replacements, changes, "fixture.one", "fixture.two", "Версия урока: 1 -> 2", "ba82-n002", "ba82-n002-v02")
			if available {
				previewTestContains(t, output.String(), preview.Resources(snapshot.Resources), "Всего файлов: 2", "Использовано: 1", "Не использовано: 1")
			} else {
				previewTestAbsent(t, output.String(), "Область: ресурсы", "Всего файлов:")
			}
			previewTestAbsent(t, output.String(), "INSTRUCTION TEXT Z", "INSTRUCTION TEXT A", "EXAMPLE TEXT")
			archiveLine := "Архив: " + strconv.Quote(snapshot.ArchivePath) + "\n"
			previewTestContains(t, output.String(), archiveLine, `Подзаголовок: "Subtitle"`)
			// Полный путь содержит имя этого теста с Prepared; это не metadata.
			previewTestAbsent(t, strings.ReplaceAll(output.String(), archiveLine, ""), "Prepared")
		})
	}
}

func TestConsolePreviewCompositionUsesWorkingCatalogBeforeManifestReady(t *testing.T) {
	op, _ := previewTestCreate(t, 2)
	snapshot := op.Snapshot()
	// Неполный typed Snapshot проверяет только проекцию, не достижимость app.
	snapshot.ManifestReady = false
	snapshot.Module.Manifest = manifest.Manifest{Course: manifest.Course{Title: "STALE-MANIFEST"}}
	snapshot.Catalog.Sectioned = true
	snapshot.Catalog.Sections = []catalog.SectionOrder{{Title: "Сначала Z", Weight: previewTestString("0002")}, {Title: "Пустая A"}}
	snapshot.Catalog.Items[0].Section = previewTestString("Сначала Z")
	snapshot.Catalog.Items[0].Entry.Order = previewTestString("неверный вес")
	snapshot.Catalog.Items[1].Section = nil
	snapshot.Catalog.Items[1].Entry.Order = nil
	snapshot.NewFileNames = []string{"l002.json"}
	before, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := previewTestDialog(&output, "").showComposition(snapshot); err != nil {
		t.Fatal(err)
	}
	previewTestContains(t, output.String(), "Курс preview", "Юнит preview", "1. \"Сначала Z\"", "2. \"Пустая A\"", "0002", "(пустая рабочая секция)", "Не распределены", "неверный вес", "order: не задан", "НОВЫЙ | ID: \"BA83\"", "Итого: уроков 2; страниц 2; нод 14.", "Строк: 23")
	previewTestAbsent(t, output.String(), "STALE-MANIFEST", "Версия урока:", "INSTRUCTION TEXT A")
	if strings.Index(output.String(), "Сначала Z") >= strings.Index(output.String(), "Пустая A") {
		t.Fatal("working section order was sorted again")
	}
	after, err := json.Marshal(snapshot)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("presentation mutated typed snapshot: %v", err)
	}
}

func TestConsolePreviewCompositionShowsAllConflictsAfterCompositionError(t *testing.T) {
	op, _ := previewTestCreate(t, 3)
	snapshot := op.Snapshot()
	snapshot.Catalog.Items[0].Entry.Order = previewTestString("0001")
	snapshot.Catalog.Items[1].Entry.Order = previewTestString("01")
	snapshot.Catalog.Items[2].Entry.Order = previewTestString("1")
	snapshot.Conflicts = catalog.Conflicts(snapshot.Catalog)
	if len(snapshot.Conflicts) != 1 {
		t.Fatal("owner must supply conflict fixture")
	}
	// Несогласованность селектора намеренно существует только в проекции.
	snapshot.Module.Files = snapshot.Module.Files[:1]
	var output bytes.Buffer
	err := previewTestDialog(&output, "").showComposition(snapshot)
	previewTestPresentation(t, err)
	previewTestContains(t, output.String(), preview.OrderConflicts(snapshot.Conflicts), "ОШИБКА СБОРКИ", "l001.json", "l002.json", "l003.json", "0001", "01", snapshot.Catalog.Items[0].Context.Path, snapshot.Catalog.Items[1].Context.Path)
}

func TestConsolePreviewSnapshotSkipsCompositionWithEitherMissingRequiredModel(t *testing.T) {
	for _, missing := range []string{"catalog", "module"} {
		t.Run(missing, func(t *testing.T) {
			snapshot := previewTestReadyUpdate(t).Snapshot()
			snapshot.Mode = app.Create
			if missing == "catalog" {
				snapshot.Catalog = nil
			} else {
				snapshot.Module = nil
			}
			snapshot.ManifestChanges = []catalog.Change{{Field: "course.title", Before: json.RawMessage(`"Прежний"`), After: json.RawMessage(`"Доступная правка"`)}}
			snapshot.Blockers = []*diagnostic.Error{{Code: "fixture.partial", Reason: "Доступная диагностика"}}
			var output bytes.Buffer
			if err := previewTestDialog(&output, "").showSnapshot(snapshot); err != nil {
				t.Fatal(err)
			}
			previewTestContains(t, output.String(), "Доступная правка", "fixture.partial", "Доступная диагностика")
		})
	}
}
