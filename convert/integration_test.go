package convert_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func integrationAt() time.Time {
	return time.Date(2026, 11, 7, 15, 15, 45, 987654321, time.FixedZone("test", 5*3600+45*60))
}

func integrationRead(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func integrationJSON(t *testing.T, value any) []byte {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func integrationLoad(t *testing.T, tree *filetree.Resolver, path string) *parsed.Input {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, issue := tree.Inspect(absolute)
	if issue != nil {
		t.Fatalf("Inspect: %+v", issue)
	}
	beforeFile := integrationRead(t, entry.Path)
	input, issues := parsed.Load(entry.Path)
	if len(issues) != 0 || input == nil {
		t.Fatalf("Load(%s): %v", path, issues)
	}
	beforeInput := integrationJSON(t, input)
	t.Cleanup(func() {
		if !bytes.Equal(beforeInput, integrationJSON(t, input)) {
			t.Errorf("Build изменил Input, индекс или вложенные данные %s", path)
		}
		if !bytes.Equal(beforeFile, integrationRead(t, entry.Path)) {
			t.Errorf("Изменён исходный JSON %s", path)
		}
	})
	return input
}

func integrationFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "integration"))); err != nil {
		t.Fatal(err)
	}
	return dir
}

func integrationExpected(t *testing.T, name string) *lesson.Lesson {
	t.Helper()
	var expected lesson.Lesson
	if err := json.Unmarshal(integrationRead(t, filepath.Join("testdata", "integration", name)), &expected); err != nil {
		t.Fatal(err)
	}
	if issues := lesson.Validate(&expected, diagnostic.Context{}); len(issues) != 0 {
		t.Fatalf("Ошибочный независимый эталон %s: %v", name, issues)
	}
	return &expected
}

func integrationEqual(t *testing.T, got, want *lesson.Lesson) {
	t.Helper()
	if !bytes.Equal(integrationJSON(t, got), integrationJSON(t, want)) {
		t.Fatalf("Модель отличается\nполучено: %s\nожидается: %s", integrationJSON(t, got), integrationJSON(t, want))
	}
}

func integrationBuild(t *testing.T, input *parsed.Input, tree *filetree.Resolver) (*lesson.Lesson, *resourceusage.Registry) {
	t.Helper()
	value, usage, issues := convert.Build(input, tree, integrationAt(), diagnostic.Context{
		FileName: "l017.json", Path: "old.zip", LessonID: "OLD", PageID: "old-p099", Line: 999, Column: 888,
	})
	if issues != nil || value == nil || usage == nil {
		t.Fatalf("Build: value=%v usage=%v errors=%v", value, usage, issues)
	}
	context := diagnostic.Context{Path: input.Path, FileName: "l017.json"}
	if issues := lesson.Validate(value, context); len(issues) != 0 {
		t.Fatalf("Успешный Build вернул невалидный урок: %v", issues)
	}
	if issues := lesson.ValidateIDs(value, context); len(issues) != 0 {
		t.Fatalf("Неверные ID первичной версии: %v", issues)
	}
	if value.Document.CreatedAt != "2026-11-07T09:30:45Z" || value.Document.Version.String() != "1" {
		t.Fatalf("Неверные дата UTC/первичная версия: %+v", value.Document)
	}
	return value, usage
}

func TestIntegrationOriginalGoldenModels(t *testing.T) {
	var tree filetree.Resolver
	for _, n := range []int{2, 4, 5, 6, 7} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			input := integrationLoad(t, &tree, filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", n)))
			value, usage := integrationBuild(t, input, &tree)
			integrationEqual(t, value, integrationExpected(t, fmt.Sprintf("golden-%d-tz.expected.json", n)))
			report := usage.Snapshot()
			if report.Total != 0 || report.Used != 0 || report.Unused != 0 || len(report.UsedPaths)+len(report.UnusedPaths) != 0 {
				t.Fatalf("Немедийный golden дал ресурсы: %+v", report)
			}
		})
	}
}
