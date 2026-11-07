package filetree

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// Entry — пассивное представление. Эти проверки фиксируют пригодность API
// для потребителей, но не проверяют, как Inspect получает поля структуры.
func TestEntryKeepsDiscoveryNameApartFromTargetMetadata(t *testing.T) {
	root := t.TempDir()
	discoveryDir := filepath.Join(root, "Вход JSON")
	targetDir := filepath.Join(root, "Общие ресурсы")
	for _, directory := range []string{discoveryDir, targetDir} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	targetPath := filepath.Join(targetDir, "Сведения цели.bin")
	if err := os.WriteFile(targetPath, []byte("not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	discoveryPath := filepath.Join(discoveryDir, "Урок.JSON")
	if err := os.Link(targetPath, discoveryPath); err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}

	// Первый адрес объекта может быть закреплён за IdentityPath, а последующее
	// обращение с другим именем должно помещаться в ту же модель независимо.
	entry := Entry{Path: discoveryPath, IdentityPath: targetPath, Info: targetInfo}
	if got := filepath.Base(entry.Path); got != "Урок.JSON" {
		t.Errorf("discovery name = %q, want Урок.JSON", got)
	}
	if got := filepath.Dir(entry.Path); got != discoveryDir {
		t.Errorf("resource base = %q, want %q", got, discoveryDir)
	}
	if got := entry.Info.Name(); got != "Сведения цели.bin" {
		t.Errorf("target metadata name = %q, want Сведения цели.bin", got)
	}
	if entry.Info.IsDir() || !entry.Info.Mode().IsRegular() {
		t.Errorf("target kind = %v, want a regular file", entry.Info.Mode())
	}

	// IdentityPath остаётся доступным путём, а стандартные сведения позволяют
	// сопоставить объект через os.SameFile без строкового сравнения имён.
	for _, path := range []string{entry.Path, entry.IdentityPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %q: %v", path, err)
		}
		if !os.SameFile(entry.Info, info) {
			t.Errorf("%q does not identify the metadata target", path)
		}
	}
}

func TestEntryAcceptsStandardDirectoryFileInfo(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "Каталог с пробелами")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	entry := Entry{Path: directory, IdentityPath: directory, Info: info}

	// Присваивание фиксирует совместимость поля со стандартным io/fs API;
	// каталог должен классифицироваться через сведения, а не по имени пути.
	var target fs.FileInfo = entry.Info
	if !target.IsDir() || target.Mode()&fs.ModeDir == 0 {
		t.Errorf("target kind = %v, want a directory", target.Mode())
	}
	if !os.SameFile(target, info) {
		t.Error("standard directory metadata lost its filesystem identity")
	}
}
