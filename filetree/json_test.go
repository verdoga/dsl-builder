package filetree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func qa17Write(t *testing.T, root, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func qa17Error(t *testing.T, err *diagnostic.Error, code, path string, technical bool) {
	t.Helper()
	if err == nil {
		t.Fatal("ожидалась диагностика", code, path)
	}
	if err.Code != code || err.Context.Path != path {
		t.Errorf("диагностика = %#v; нужны код %q, путь %q", err, code, path)
	}
	if !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("нет русской причины: %q", err.Reason)
	}
	if err.Context.Line != 0 || err.Context.Column != 0 {
		t.Errorf("выдуманы координаты: %#v", err.Context)
	}
	if technical && err.Cause == nil {
		t.Error("утрачена исходная техническая ошибка")
	}
}

func TestQA17IsJSONUsesOnlyAddressExtension(t *testing.T) {
	root := t.TempDir()
	target := qa17Write(t, root, "target.json", nil)
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	for mask := 0; mask < 16; mask++ {
		ext := []byte("json")
		for i := range ext {
			if mask&(1<<i) != 0 {
				ext[i] -= 'a' - 'A'
			}
		}
		path := filepath.Join(root, "не существует."+string(ext))
		if !isJSON(Entry{Path: path}) {
			t.Errorf("не принято ASCII расширение: %q", path)
		}
	}
	for _, name := range []string{"x", "x.jsonc", "x.json.txt", "x.json.", "x.json ", "json", "x.ｊｓｏｎ", "dir.json/file"} {
		if isJSON(Entry{Path: filepath.Join(root, filepath.FromSlash(name)), IdentityPath: target, Info: info}) {
			t.Errorf("расширение взято не из адреса: %q", name)
		}
	}
}

func TestQA17JSONFilesRecursionOrderAndUnparsedContents(t *testing.T) {
	root := t.TempDir()
	names := []string{"a.json/child.JSON", "B.JSON", "deep" + strings.Repeat("/level space", 40) + "/lesson.JSON", "U1/DV/Work.json", "U1/SB/A.json", "U1/VOX/Work.json", "U1/WB/A.json", "кириллица с пробелом/урок.JsOn"}
	contents := [][]byte{nil, {0xff, 0x00}, []byte("{}"), []byte("not JSON"), []byte("null"), []byte("[]"), []byte(`{"formatVersion":"999"}`), []byte(`{"document":{"hasErrors":true}}`)}
	for i, name := range names {
		path := qa17Write(t, root, name, contents[i])
		stamp := time.Unix(int64(1000-i), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"readme.txt", "x.jsonc", "U1/SB/Audio/media.mp3", "U1/WB/Audio/sound.wav", "U1/DV/Video/movie.mp4", "U1/VOX/Video/movie.mp4"} {
		qa17Write(t, root, name, []byte("{}"))
	}
	var resolver Resolver
	for attempt := 0; attempt < 2; attempt++ {
		got, err := resolver.JSONFiles(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(names) {
			t.Fatalf("найдено %d файлов, нужно %d: %#v", len(got), len(names), got)
		}
		for i, entry := range got {
			if entry.Path != filepath.Join(root, filepath.FromSlash(names[i])) || entry.IdentityPath == "" || entry.Info == nil || !entry.Info.Mode().IsRegular() {
				t.Errorf("запись %d: %#v; нужен %q", i, entry, names[i])
			}
		}
	}
}

func TestQA17JSONFilesCaseSensitiveTieOnLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		root := t.TempDir()
		lower := qa17Write(t, root, "a.json", nil)
		upper := qa17Write(t, root, "A.JSON", nil)
		var resolver Resolver
		got, err := resolver.JSONFiles(root)
		if err != nil || len(got) != 2 || got[0].Path != upper || got[1].Path != lower {
			t.Fatalf("дополнительный ключ исходного регистра: %#v, %v", got, err)
		}
	}
}

func TestQA17JSONFileAcceptsBytesWithoutValidation(t *testing.T) {
	root := t.TempDir()
	for i, content := range [][]byte{nil, {0xff}, []byte("{"), []byte("null"), []byte(`{"formatVersion":"0","document":{"hasErrors":true}}`)} {
		path := qa17Write(t, root, string(rune('a'+i))+".JsON", content)
		var resolver Resolver
		entry, err := resolver.JSONFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if entry.Path != path || entry.Info == nil || !entry.Info.Mode().IsRegular() || entry.Info.Size() != int64(len(content)) {
			t.Errorf("неверные метаданные: %#v", entry)
		}
	}
}

func TestQA17JSONInputFailures(t *testing.T) {
	root := t.TempDir()
	file := qa17Write(t, root, "ordinary.txt", []byte("{}"))
	directory := filepath.Join(root, "directory.json")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	var resolver Resolver
	for _, tc := range []struct{ path, code string }{{"", CodePath}, {filepath.Join(root, "missing.json"), CodeAccess}, {file, CodeExtension}, {directory, CodeKind}} {
		t.Run(tc.code, func(t *testing.T) {
			_, err := resolver.JSONFile(tc.path)
			qa17Error(t, err, tc.code, tc.path, tc.code == CodeAccess)
			if tc.code == CodeAccess && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("не сохранена ошибка отсутствия: %v", err)
			}
		})
	}
	for _, tc := range []struct{ path, code string }{{"", CodePath}, {filepath.Join(root, "missing"), CodeAccess}, {file, CodeKind}, {directory, CodeEmptyJSON}, {root, CodeEmptyJSON}} {
		got, err := resolver.JSONFiles(tc.path)
		qa17Error(t, err, tc.code, tc.path, tc.code == CodeAccess)
		if got != nil {
			t.Errorf("частичный/пустой результат вместо nil: %#v", got)
		}
	}
	if runtime.GOOS == "linux" {
		_, err := resolver.JSONFile(os.DevNull)
		qa17Error(t, err, CodeKind, os.DevNull, false)
	}
}

func TestQA17JSONAliasesFilterBeforeUnique(t *testing.T) {
	for _, hard := range []bool{false, true} {
		t.Run(map[bool]string{false: "symlink", true: "hardlink"}[hard], func(t *testing.T) {
			root := t.TempDir()
			target := qa17Write(t, root, "a.bin", []byte("invalid JSON"))
			alias := filepath.Join(root, "z.JSON")
			link := os.Symlink
			if hard {
				link = os.Link
			}
			if err := link(target, alias); err != nil {
				t.Fatal(err)
			}
			if err := link(target, filepath.Join(root, "zz.json")); err != nil {
				t.Fatal(err)
			}
			var resolver Resolver
			first, err := resolver.Inspect(target)
			if err != nil {
				t.Fatal(err)
			}
			entry, err := resolver.JSONFile(alias)
			if err != nil || entry.Path != alias || entry.IdentityPath != first.IdentityPath {
				t.Fatalf("JSONFile псевдонима: %#v, %v", entry, err)
			}
			got, err := resolver.JSONFiles(root)
			if err != nil || len(got) != 1 || got[0].Path != alias || got[0].IdentityPath != first.IdentityPath {
				t.Fatalf("отбор до Unique: %#v, %v", got, err)
			}
		})
	}
	root := t.TempDir()
	target := qa17Write(t, root, "target.json", nil)
	alias := filepath.Join(root, "alias.bin")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	var resolver Resolver
	_, err := resolver.JSONFile(alias)
	qa17Error(t, err, CodeExtension, alias, false)
}

func TestQA17JSONFilesNeverReturnsPartialResult(t *testing.T) {
	for _, loop := range []bool{false, true} {
		t.Run(map[bool]string{false: "dangling", true: "loop"}[loop], func(t *testing.T) {
			root := t.TempDir()
			qa17Write(t, root, "a.json", nil)
			bad := filepath.Join(root, "z-bad.json")
			target := filepath.Join(root, "missing")
			if loop {
				target = bad
			}
			if err := os.Symlink(target, bad); err != nil {
				t.Fatal(err)
			}
			var resolver Resolver
			got, err := resolver.JSONFiles(root)
			qa17Error(t, err, CodeLink, bad, true)
			if got != nil {
				t.Errorf("возвращён частичный результат: %#v", got)
			}
			_, err = resolver.JSONFile(bad)
			qa17Error(t, err, CodeLink, bad, true)
		})
	}
}

func TestQA17JSONMetadataWithoutReadAndAccessFailure(t *testing.T) {
	if runtime.GOOS == "linux" {
		root := t.TempDir()
		file := qa17Write(t, root, "a.json", []byte("not JSON"))
		if err := os.Chmod(file, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := os.ReadFile(file); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("для проверки нужен запрет чтения байтов: %v", err)
		}
		var resolver Resolver
		if _, err := resolver.JSONFile(file); err != nil {
			t.Fatalf("метаданные не требуют чтения содержимого: %v", err)
		}
		if got, err := resolver.JSONFiles(root); err != nil || len(got) != 1 {
			t.Fatalf("поиск не требует чтения содержимого: %#v, %v", got, err)
		}
		blocked := filepath.Join(root, "z-blocked")
		child := qa17Write(t, root, "z-blocked/child.json", nil)
		if err := os.Chmod(blocked, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(blocked, 0700); err != nil {
				t.Error(err)
			}
		})
		for _, path := range []string{root, blocked} {
			got, err := resolver.JSONFiles(path)
			qa17Error(t, err, CodeAccess, blocked, true)
			if got != nil || !errors.Is(err, fs.ErrPermission) {
				t.Errorf("ошибка доступа обязана запретить результат: %#v, %v", got, err)
			}
		}
		_, err := resolver.JSONFile(child)
		qa17Error(t, err, CodeAccess, child, true)
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("не сохранена причина отказа доступа: %v", err)
		}
	}
}
