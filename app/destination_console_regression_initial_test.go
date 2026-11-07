package app_test

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestDestinationConsoleRegressionInitialCreateStripsOnePair(t *testing.T) {
	for _, test := range []struct {
		name      string
		directory string
		argument  string
		quoteDir  bool
	}{
		{name: "ordinary raw quoted input", directory: "Источник 世界", argument: `"Источник 世界"`},
		{name: "one literal pair remains", directory: `"source"`, argument: `""source""`, quoteDir: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.quoteDir && runtime.GOOS == "windows" {
				t.Skip("fixture с буквальными кавычками в имени каталога проверяется на Linux")
			}
			root := t.TempDir()
			source := destinationConsoleRegressionSource(t, root, test.directory)
			op := destinationConsoleRegressionNewCreate(t, root, test.argument)
			files := op.Snapshot().Module.Files
			if len(files) != 1 || files[0].Context().Path != filepath.Join(source, "source.json") {
				t.Fatal("начальный путь не снял ровно одну пару либо потерял Unicode/пробелы")
			}
		})
	}
}

func TestDestinationConsoleRegressionInitialArchiveConstructorsKeepQuoteHandling(t *testing.T) {
	for _, test := range []struct {
		name        string
		constructor func(string, string, string) (*app.Operation, []*diagnostic.Error)
	}{
		{name: "update", constructor: app.NewUpdate},
		{name: "add", constructor: app.NewAdd},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			destinationConsoleRegressionSource(t, root, "Источник с пробелами")
			before := destinationConsoleRegressionInventory(t, root)
			// Конструктор только сохраняет параметры; отсутствующий ZIP не читается.
			op, issues := test.constructor(root, `"Модуль 世界.zip"`, `"Источник с пробелами/source.json"`)
			destinationConsoleRegressionNoIssues(t, issues)
			if op == nil {
				t.Fatal("конструктор вернул nil")
			}
			t.Cleanup(op.Cancel)
			want := destinationConsoleRegressionTarget(t, root, ".", "Модуль 世界.zip", false)
			if op.Snapshot().ArchivePath != want.Path {
				t.Fatalf("исходный ZIP: %q, нужен %q", op.Snapshot().ArchivePath, want.Path)
			}
			if after := destinationConsoleRegressionInventory(t, root); !slices.Equal(before, after) {
				t.Fatal("конструктор изменил файлы")
			}
		})
	}
}
