package resourceembed

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourcefind"
)

func task6Find(t *testing.T, tree *filetree.Resolver, dir, kind, name string) (resourcefind.Selection, diagnostic.Context) {
	t.Helper()
	context := diagnostic.Context{Path: filepath.Join(dir, "Исходник.json"), FileName: "l001.json", LessonID: "EFX7", PageID: "efx7-p001", Line: 35, Column: 14}
	finder, _, problem := resourcefind.Prepare(tree, context.Path, nil, context)
	if problem != nil {
		t.Fatalf("Prepare: %v", problem)
	}
	selection, problem := finder.Find(kind, `"`+name+`"`, name, context)
	if problem != nil {
		t.Fatalf("Find: %v", problem)
	}
	return selection, context
}

func task6Write(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
}

func task6ReadFailure(t *testing.T, file resourcefind.File, context diagnostic.Context) {
	t.Helper()
	got, problem := readBytes(file, context)
	if got != nil || problem == nil {
		t.Fatalf("readBytes = %x, %v; want nil bytes and error", got, problem)
	}
	if problem.Code != CodeRead || problem.Reason != "Не удалось прочитать выбранный ресурс" || problem.Context.Path != file.Entry.Path || problem.Cause == nil {
		t.Fatalf("incorrect read diagnostic: %#v", problem)
	}
	found := false
	for _, related := range problem.Related {
		found = found || related == context
	}
	if !found {
		t.Fatalf("missing original context: %#v", problem.Related)
	}
}

func TestTask6ReadBytesPreservesSelectedOpaqueFiles(t *testing.T) {
	dir := t.TempDir()
	allBytes := make([]byte, 8193)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	names := []string{"Трек 01.MP4", "Трек 01.txt", "Трек 01.vtt", "Трек 01.srt", "Трек 01.ttml", "Трек 01.dfxp", "Трек 01.ass", "Трек 01.ssa"}
	payloads := [][]byte{allBytes, {0xef, 0xbb, 0xbf, 'A', '\r', '\n', 0xff, 0}, {}, {'\r', '\n', '\r', '\n'}, {0xff, 0xfe, 0, 'A'}, {0xc3, 0x28}, {0, 0, 0}, []byte("не контейнер\n")}
	for i, name := range names {
		task6Write(t, filepath.Join(dir, name), payloads[i])
	}
	selection, context := task6Find(t, &filetree.Resolver{}, dir, "video", names[0])
	if selection.Script == nil || len(selection.Subtitles) != 6 {
		t.Fatal("real Find did not select all companions")
	}
	files := append([]resourcefind.File{selection.Body, *selection.Script}, selection.Subtitles...)
	for i, file := range files {
		t.Run(names[i], func(t *testing.T) {
			got, problem := readBytes(file, context)
			if problem != nil || !bytes.Equal(got, payloads[i]) {
				t.Fatalf("readBytes changed opaque bytes: %x, %v", got, problem)
			}
			disk, err := os.ReadFile(file.Entry.Path)
			if err != nil || !bytes.Equal(disk, payloads[i]) {
				t.Fatalf("input file changed: %x, %v", disk, err)
			}
		})
	}
}

func TestTask6ReadBytesUsesCurrentContentsAfterFind(t *testing.T) {
	for _, replacement := range [][]byte{{}, {0xff}, bytes.Repeat([]byte{0, 0xfe, '\r', '\n'}, 4097)} {
		dir := t.TempDir()
		path := filepath.Join(dir, "Изображение.PNG")
		task6Write(t, path, []byte("old metadata"))
		selection, context := task6Find(t, &filetree.Resolver{}, dir, "image", filepath.Base(path))
		task6Write(t, path, replacement)
		got, problem := readBytes(selection.Body, context)
		if problem != nil || !bytes.Equal(got, replacement) {
			t.Fatalf("stale Info used instead of actual bytes: %x, %v", got, problem)
		}
	}
}

func TestTask6ReadBytesFailsAfterSelectedFileDisappearsOrBecomesDirectory(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "deleted", true: "directory"}[directory], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "Медиа.png")
			task6Write(t, path, []byte("selected"))
			selection, context := task6Find(t, &filetree.Resolver{}, dir, "image", filepath.Base(path))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if directory {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			task6ReadFailure(t, selection.Body, context)
			if !directory {
				_, problem := readBytes(selection.Body, context)
				if !errors.Is(problem, fs.ErrNotExist) {
					t.Fatalf("lost filesystem cause: %v", problem)
				}
			}
		})
	}
}

func TestTask6SelectedLinkKeepsAddressNameAndHasNoIdentityFallback(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Цель.png")
	alias := filepath.Join(dir, "Имя Ссылки.PNG")
	payload := []byte{0, 0xff, 0xef, 0xbb, 0xbf}
	task6Write(t, target, payload)
	if err := os.Symlink(target, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("filesystem symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	tree := &filetree.Resolver{}
	if _, problem := tree.Inspect(target); problem != nil {
		t.Fatal(problem)
	}
	selection, context := task6Find(t, tree, dir, "image", filepath.Base(alias))
	file := selection.Body
	if file.Entry.Path != alias || file.Entry.IdentityPath != target || file.Entry.Info.Name() == filepath.Base(alias) {
		t.Fatalf("fixture lacks distinct address, identity and info name: %#v", file.Entry)
	}
	got, problem := readBytes(file, context)
	if problem != nil || !bytes.Equal(got, payload) || embeddedFile(file, got).FileName != filepath.Base(alias) {
		t.Fatalf("selected alias read/name failed: %x, %v", got, problem)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	task6ReadFailure(t, file, context)
	if disk, err := os.ReadFile(target); err != nil || !bytes.Equal(disk, payload) {
		t.Fatalf("identity fallback target must remain readable: %x, %v", disk, err)
	}
}

func TestTask6EmbeddedFileWindowsNamesAndExactBase64(t *testing.T) {
	// Лексические Windows-адреса проверяют private helper без файлового ввода.
	paths := []string{`C:\Каталог с пробелом\ИмЯ 01.MP4`, `\\сервер\общий каталог\ИмЯ 01.MP4`, `\\?\C:\длинный путь\ИмЯ 01.MP4`, `C:/Каталог/ИмЯ 01.MP4`, `C:\Каталог/Вложенный\ИмЯ 01.MP4`}
	for _, path := range paths {
		file := resourcefind.File{Entry: filetree.Entry{Path: path, IdentityPath: `C:\Цель\другое.png`}, MIMEType: "Application/Custom; charset=opaque"}
		for _, payload := range [][]byte{nil, {}, {0}, {0xfb, 0xff}, {0xfb, 0xff, 0xff}, bytes.Repeat([]byte{0, 0xff, '\r', '\n'}, 64)} {
			original := bytes.Clone(payload)
			got := embeddedFile(file, payload)
			if got.FileName != "ИмЯ 01.MP4" || got.MIMEType != file.MIMEType || got.Content != base64.StdEncoding.EncodeToString(original) {
				t.Fatalf("embeddedFile(%q) = %#v", path, got)
			}
			decoded, err := base64.StdEncoding.DecodeString(got.Content)
			if err != nil || !bytes.Equal(decoded, original) || !bytes.Equal(payload, original) || strings.ContainsAny(got.Content, "\r\n") {
				t.Fatalf("Base64/ownership mismatch: %#v, %v", got, err)
			}
			for i := range payload {
				payload[i] ^= 0xff
			}
			if got.Content != base64.StdEncoding.EncodeToString(original) {
				t.Fatal("result retained input bytes")
			}
		}
	}
}

func TestTask6MediaDataHasIndependentRoleAndValue(t *testing.T) {
	value := lesson.EmbeddedFile{FileName: "Файл.mp4", MIMEType: "video/mp4", Content: "AA=="}
	roles := []lesson.DataRole{lesson.RoleBody, lesson.RoleScript, lesson.RoleSubtitles, lesson.RoleBody}
	results := make([]lesson.Data, len(roles))
	for i, role := range roles {
		results[i] = mediaData(value, role)
		got := results[i]
		embedded, ok := got.Value.(lesson.EmbeddedFile)
		if got.Type != lesson.DataMedia || got.Role == nil || *got.Role != role || !ok || embedded != value || got.Answers == nil || len(got.Answers) != 0 || got.Hint != "" {
			t.Fatalf("incorrect media Data: %#v", got)
		}
		for _, previous := range results[:i] {
			if previous.Role == got.Role {
				t.Fatal("media elements share mutable role pointer")
			}
		}
	}
	*results[0].Role = lesson.RoleTitle
	value.FileName = "changed"
	for i, got := range results[1:] {
		if *got.Role != roles[i+1] || got.Value.(lesson.EmbeddedFile).FileName != "Файл.mp4" {
			t.Fatal("value or role was retained by reference")
		}
	}
	encoded, err := json.Marshal(results[1])
	if err != nil || !bytes.Contains(encoded, []byte(`"answers":[]`)) || !bytes.Contains(encoded, []byte(`"hint":""`)) {
		t.Fatalf("required empty JSON fields missing: %s, %v", encoded, err)
	}
}

func TestTask6ReadBytesClosesOpenedDescriptors(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("descriptor observation uses Linux /proc/self/fd")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "Дескриптор.png")
	task6Write(t, path, []byte{0xff, 0})
	selection, context := task6Find(t, &filetree.Resolver{}, dir, "image", filepath.Base(path))
	for _, failure := range []bool{false, true} {
		if failure {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}
		for range 16 {
			got, problem := readBytes(selection.Body, context)
			if failure && (got != nil || problem == nil) || !failure && (problem != nil || !bytes.Equal(got, []byte{0xff, 0})) {
				t.Fatalf("readBytes failure=%v: %x, %v", failure, got, problem)
			}
		}
		descriptors, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		for _, descriptor := range descriptors {
			target, err := os.Readlink(filepath.Join("/proc/self/fd", descriptor.Name()))
			if err == nil && (target == path || target == path+" (deleted)") {
				t.Fatalf("readBytes retained descriptor %s to %q", descriptor.Name(), target)
			}
		}
	}
}
