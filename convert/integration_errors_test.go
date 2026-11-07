package convert_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func integrationFailure(t *testing.T, input *parsed.Input, tree *filetree.Resolver, at time.Time) (*lesson.Lesson, *resourceusage.Registry, []*diagnostic.Error) {
	t.Helper()
	value, usage, issues := convert.Build(input, tree, at, diagnostic.Context{
		FileName: "l017.json", Path: "old.zip", LessonID: "OLD", PageID: "old-p099", Line: 999, Column: 888,
	})
	if value != nil || usage != nil || len(issues) == 0 {
		t.Fatalf("Ошибка не атомарна: lesson=%v usage=%v errors=%v", value, usage, issues)
	}
	for _, issue := range issues {
		if issue == nil || issue.Code == "" || issue.Reason == "" {
			t.Fatalf("Неполная диагностика: %+v", issue)
		}
	}
	return value, usage, issues
}

func TestIntegrationOriginalMediaGoldenRequireDeclaredSearchScope(t *testing.T) {
	for _, tc := range []struct {
		n, line int
		id      string
		nearby  string
	}{
		{1, 35, "EFX7", "12.01.mp3"},
		{3, 36, "BA83", "003.mp4"},
	} {
		t.Run(fmt.Sprint(tc.n), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, fmt.Sprintf("original-golden-%d.parsed.json", tc.n))
			// Побайтовая копия оригинала; соседний файл не создаёт область поиска.
			if err := os.WriteFile(path, integrationRead(t, filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", tc.n))), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, tc.nearby), []byte{0, 255, 42}, 0600); err != nil {
				t.Fatal(err)
			}
			var tree filetree.Resolver
			input := integrationLoad(t, &tree, path)
			if len(input.Result.Document.Metadata.ResourceDirs) != 0 {
				t.Fatal("Исходный golden неожиданно объявляет resourceDirs")
			}
			_, _, issues := integrationFailure(t, input, &tree, integrationAt())
			if len(issues) != 1 || issues[0].Code != resourcefind.CodeNotFound {
				t.Fatalf("Ожидается resourcefind.not_found по ТЗ 9.1: %v", issues)
			}
			wantContext := diagnostic.Context{Path: input.Path, FileName: "l017.json", LessonID: tc.id, Line: tc.line, Column: 14}
			if issues[0].Context != wantContext {
				t.Fatalf("Неверные исходные координаты: %+v, ожидались %+v", issues[0].Context, wantContext)
			}
		})
	}
}

func TestIntegrationEarlyFailuresKeepStageAndOwnerOrder(t *testing.T) {
	root := integrationFixture(t)
	var tree filetree.Resolver
	for _, field := range []string{"title", "lineCount"} {
		t.Run("metadata-"+field+"-before-fragments-and-resources", func(t *testing.T) {
			// Новый JSON подготовлен до Load; проверенный Input не редактируется.
			var source model.Result
			if err := json.Unmarshal(integrationRead(t, filepath.Join(root, "synthetic-missing-fragments.parsed.json")), &source); err != nil {
				t.Fatal(err)
			}
			if field == "title" {
				source.Document.Metadata.Title = nil
			} else {
				source.Document.LineCount = nil
			}
			path := filepath.Join(root, "synthetic-missing-"+field+".parsed.json")
			if err := os.WriteFile(path, integrationJSON(t, source), 0600); err != nil {
				t.Fatal(err)
			}
			input := integrationLoad(t, &tree, path)
			_, _, issues := integrationFailure(t, input, &tree, integrationAt())
			if len(issues) != 1 || issues[0].Code != convert.CodeMetadata {
				t.Fatalf("Ожидается ранняя ошибка metadata: %v", issues)
			}
			// Контракт требует имя поля, но не закрепляет носитель в диагностике.
			namesField := func(text string) bool {
				return slices.Contains(strings.FieldsFunc(text, func(r rune) bool {
					return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
				}), field)
			}
			if !namesField(issues[0].Reason) && !slices.ContainsFunc(issues[0].Details, func(d diagnostic.Detail) bool {
				return namesField(d.Label) || namesField(d.Value)
			}) {
				t.Fatalf("Не названо отсутствующее поле %s: reason=%q details=%+v", field, issues[0].Reason, issues[0].Details)
			}
			want := diagnostic.Context{Path: input.Path, FileName: "l017.json", LessonID: "Mix-A9"}
			if issues[0].Context != want {
				t.Fatalf("Старый контекст перенесён в новый вход: %+v", issues[0].Context)
			}
		})
	}
	t.Run("all-fragment-errors-before-missing-directory", func(t *testing.T) {
		input := integrationLoad(t, &tree, filepath.Join(root, "synthetic-missing-fragments.parsed.json"))
		_, _, first := integrationFailure(t, input, &tree, integrationAt())
		_, _, second := integrationFailure(t, input, &tree, integrationAt())
		for _, issues := range [][]*diagnostic.Error{first, second} {
			if len(issues) != 2 {
				t.Fatalf("Потеря ошибок владельца fragments: %v", issues)
			}
			for i, name := range []string{"absent-a", "absent-b"} {
				issue := issues[i]
				if issue.Code != fragments.CodeMissing || issue.Context.Path != input.Path || issue.Context.LessonID != "Mix-A9" || issue.Context.Line != i+1 || issue.Context.Column != 10 || issue.Context.PageID != "" {
					t.Fatalf("Неверный порядок/контекст fragments[%d]: %+v", i, issue)
				}
				if !slices.ContainsFunc(issue.Details, func(d diagnostic.Detail) bool { return d.Value == name }) {
					t.Fatalf("Потеря имени фрагмента %s: %+v", name, issue.Details)
				}
			}
		}
		first[0].Details[0].Value = "изменено"
		first[0].Context.Line = 99
		if second[0].Details[0].Value != "absent-a" || second[0].Context.Line != 1 {
			t.Fatal("Повторные ошибки разделяют изменяемую память")
		}
	})
}

func TestIntegrationLateFailuresDiscardEmbeddedLessonAndUsage(t *testing.T) {
	root := integrationFixture(t)
	var tree filetree.Resolver
	t.Run("exercise-after-successful-media", func(t *testing.T) {
		input := integrationLoad(t, &tree, filepath.Join(root, "synthetic-empty-question.parsed.json"))
		_, _, issues := integrationFailure(t, input, &tree, integrationAt())
		if len(issues) != 1 || issues[0].Code != exercisedata.CodeSource {
			t.Fatalf("Ожидается ошибка данных после встраивания media: %v", issues)
		}
		want := diagnostic.Context{Path: input.Path, FileName: "l017.json", LessonID: "Mix-A9", Line: 22, Column: 11}
		if issues[0].Context != want {
			t.Fatalf("Потерян контекст проблемной привязки: %+v", issues[0].Context)
		}
	})
	t.Run("initial-validation-after-all-data", func(t *testing.T) {
		input := integrationLoad(t, &tree, filepath.Join(root, "synthetic-composite.parsed.json"))
		_, _, issues := integrationFailure(t, input, &tree, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))
		if len(issues) != 1 || issues[0].Code != lesson.CodeContract || !slices.ContainsFunc(issues[0].Details, func(d diagnostic.Detail) bool { return d.Value == "/document/createdAt" }) {
			t.Fatalf("Поздняя проверка Initial не передана: %v", issues)
		}
		// Неуспех не загрязняет следующий локальный реестр того же входа.
		value, usage := integrationBuild(t, input, &tree)
		integrationEqual(t, value, integrationExpected(t, "synthetic-composite.expected.json"))
		if report := usage.Snapshot(); report.Total != 7 || report.Used != 5 || report.Unused != 2 {
			t.Fatalf("Реестр после поздней ошибки: %+v", report)
		}
	})
}
