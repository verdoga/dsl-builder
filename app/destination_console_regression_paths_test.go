package app_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/filetree"
)

func TestDestinationConsoleRegressionLiteralDirectory(t *testing.T) {
	cases := []struct {
		name      string
		directory string
		absolute  bool
		quoteDir  bool
	}{
		{name: "plain post-frontend", directory: "out"},
		{name: "Unicode and spaces", directory: "Модули 世界 с пробелами"},
		{name: "absolute", directory: "Полный путь", absolute: true},
		{name: "remaining quote pair", directory: `"out"`, quoteDir: true},
		{name: "two remaining quote pairs", directory: `""out""`, quoteDir: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.quoteDir && runtime.GOOS == "windows" {
				t.Skip("буквальная кавычка в имени каталога допустима на Linux; Windows не допускает такой fixture")
			}
			root := t.TempDir()
			source := destinationConsoleRegressionSource(t, root, "source")
			destinationConsoleRegressionDirectory(t, root, "out")
			// Даже при наличии plain out оставшиеся кавычки являются частью пути.
			directory := destinationConsoleRegressionDirectory(t, root, test.directory)
			argument := test.directory
			if test.absolute {
				argument = directory
			}
			op := destinationConsoleRegressionNewCreate(t, root, source)
			before := op.Snapshot()
			model := destinationConsoleRegressionModel(t, before)
			// Это готовые literal post-frontend аргументы, не реализация helper console.
			destinationConsoleRegressionNoIssues(t, op.SetDestination(before.Token, argument, "Модуль.ZIP"))
			after := op.Snapshot()
			want := destinationConsoleRegressionTarget(t, root, argument, "Модуль.ZIP", false)
			destinationConsoleRegressionWantTarget(t, after, want)
			if after.Token == before.Token || !after.Actions.ReviewSave || !after.Actions.ChooseDestination {
				t.Fatalf("успешная смена цели не опубликовала новый review: %+v", after)
			}
			if destinationConsoleRegressionModel(t, after) != model {
				t.Fatal("выбор цели изменил каталог, манифест, уроки либо ресурсы")
			}
			preview, issues := op.ReviewSave(after.Token)
			destinationConsoleRegressionNoIssues(t, issues)
			if preview == nil || preview.Destination != want {
				t.Fatalf("ReviewSave не показывает буквальную цель: %+v", preview)
			}
			destinationConsoleRegressionNoConsent(t, op)
		})
	}
}

func TestDestinationConsoleRegressionInvalidDirectoryPreservesTarget(t *testing.T) {
	cases := []struct {
		name      string
		directory string
	}{
		{name: "remaining pair with existing unquoted directory", directory: `"out"`},
		{name: "two remaining pairs", directory: `""out""`},
		{name: "missing parent", directory: "missing"},
		{name: "regular file parent", directory: "not-directory"},
		{name: "empty", directory: ""},
		{name: "NUL", directory: "out\x00bad"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := destinationConsoleRegressionSource(t, root, "source")
			destinationConsoleRegressionDirectory(t, root, "out")
			destinationConsoleRegressionWrite(t, filepath.Join(root, "not-directory"), []byte("родитель — файл"))
			op := destinationConsoleRegressionNewCreate(t, root, source)
			destinationConsoleRegressionNoIssues(t, op.SetDestination(op.Snapshot().Token, "out", "previous"))
			before := op.Snapshot()
			model := destinationConsoleRegressionModel(t, before)
			resolved, resolveIssue := filetree.Resolve(root, test.directory)
			code := app.CodeDestination
			if resolveIssue != nil {
				code = resolveIssue.Code
			} else if info, err := os.Stat(resolved); err == nil && info.IsDir() {
				t.Fatalf("отрицательный literal fixture неожиданно существует: %q", resolved)
			}
			issues := op.SetDestination(before.Token, test.directory, "replacement")
			destinationConsoleRegressionIssue(t, issues, code)
			after := op.Snapshot()
			destinationConsoleRegressionWantTarget(t, after, *before.Target)
			if after.Token != before.Token || len(after.Blockers) != 0 || destinationConsoleRegressionModel(t, after) != model {
				t.Fatal("неверный ввод изменил принятую модель, token либо блокировки")
			}
			if resolveIssue == nil {
				wantPath, issue := filetree.Resolve(resolved, "replacement.zip")
				if issue != nil {
					t.Fatal(issue)
				}
				if issues[0] == nil || issues[0].Context.Path != wantPath {
					t.Fatalf("диагностика потеряла буквальный путь %q: %v", wantPath, issues)
				}
			}
			destinationConsoleRegressionNoConsent(t, op)
		})
	}
}
