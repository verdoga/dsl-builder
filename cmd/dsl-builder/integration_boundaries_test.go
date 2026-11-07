package main

import (
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationCancellationAndDomainErrors(t *testing.T) {
	binary := integrationTestBuild(t)
	for _, scenario := range []string{"startup EOF", "partial path EOF", "invalid numeric choice", "domain error then EOF"} {
		t.Run(scenario, func(t *testing.T) {
			cwd := filepath.Join(t.TempDir(), "Папка с пробелами Ω")
			if err := os.Mkdir(cwd, 0755); err != nil {
				t.Fatal(err)
			}
			before := integrationTestTree(t, cwd, "")
			p := integrationTestStart(t, binary, cwd)
			switch scenario {
			case "startup EOF":
				p.until("Ваш выбор:")
			case "partial path EOF":
				p.choose("Создать новый модуль")
				p.choose("Ввести значение")
				p.until("Введите значение:")
				if _, err := p.input.Write([]byte("незавершённый путь")); err != nil {
					t.Fatal(err)
				}
			case "invalid numeric choice":
				p.until("Ваш выбор:")
				p.send("не число")
				screen := p.choose("Выход")
				if !strings.Contains(screen, "Некорректный выбор") {
					t.Fatalf("missing numeric retry: %s", screen)
				}
			case "domain error then EOF":
				p.choose("Обновить существующий модуль")
				p.value("Отсутствующий архив.zip", false)
				p.value("Отсутствующий вход.json", false)
				screen := p.until("Ваш выбор:")
				if !strings.Contains(screen, "Код:") || !strings.Contains(screen, "Отсутствующий архив.zip") || !strings.Contains(screen, "Выход") {
					t.Fatalf("missing diagnostic and safe continuation: %s", screen)
				}
			}
			output := p.finish()
			if strings.Contains(output, "Модуль успешно сохранён") || strings.Contains(output, "panic:") {
				t.Fatal("cancel/error produced success or panic")
			}
			integrationTestSameTree(t, before, integrationTestTree(t, cwd, ""))
		})
	}
}

func integrationTestTree(t *testing.T, root, exclude string) map[string][32]byte {
	t.Helper()
	files := make(map[string][32]byte)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path == exclude {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[relative] = sha256.Sum256(integrationTestRead(t, path))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func integrationTestSameTree(t *testing.T, before, after map[string][32]byte) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("unexpected files: before=%v after=%v", before, after)
	}
	for path, hash := range before {
		if actual, found := after[path]; !found || actual != hash {
			t.Fatalf("unexpected modification/removal: %s", path)
		}
	}
}
