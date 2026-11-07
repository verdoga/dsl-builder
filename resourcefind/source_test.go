package resourcefind

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

func sourceQAWrite(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0, 255, 13, 10}, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func sourceQAContext(base string) diagnostic.Context {
	return diagnostic.Context{Path: filepath.Join(base, "Урок.json"), FileName: "l007.json", LessonID: "EFx007", PageID: "EFx007.p0002", Line: 35, Column: 14}
}

func sourceQAFinder(base string, directories ...string) *Finder {
	return &Finder{tree: &filetree.Resolver{}, baseDir: base, directories: directories}
}

func sourceQAFile(t *testing.T, file File, issue *diagnostic.Error, path, mime string) {
	t.Helper()
	if issue != nil {
		t.Fatalf("выбор %q: %v", path, issue)
	}
	if file.Entry.Path != path || file.MIMEType != mime {
		t.Fatalf("выбрано %#v, ожидались %q и %q", file, path, mime)
	}
	if file.Entry.Info == nil || !file.Entry.Info.Mode().IsRegular() || file.Entry.IdentityPath == "" {
		t.Fatalf("неполный Entry: %#v", file.Entry)
	}
}

func sourceQASelectionError(t *testing.T, issue *diagnostic.Error, code string, context diagnostic.Context, kind, value string, dirs, paths []string) {
	t.Helper()
	if issue == nil {
		t.Fatal("ожидалась блокирующая диагностика")
	}
	if issue.Code != code || issue.Context != context || issue.Reason == "" {
		t.Fatalf("диагностика: %#v", issue)
	}
	if len(issue.Related) != len(paths) {
		t.Fatalf("Related: %#v; ожидались %v", issue.Related, paths)
	}
	for i, path := range paths {
		if issue.Related[i].Path != path {
			t.Errorf("кандидат %d: %q, ожидался %q", i, issue.Related[i].Path, path)
		}
	}
	for _, want := range append([]string{kind, value}, dirs...) {
		found := false
		for _, detail := range issue.Details {
			if detail.Value == want && detail.Label != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("точное значение %q отсутствует в Details: %#v", want, issue.Details)
		}
	}
}

func TestSourceQAExplicitSourceUsesOnlyBoundingQuotes(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`"track.mp3"`, true}, {`"Каталог с пробелами/003.mp4"`, true},
		{`""`, true}, {`"a\q.mp3"`, true}, {`"a"b.mp3"`, true},
		{`003`, false}, {`12.01`, false}, {`текст с пробелами`, false},
		{`folder/track.mp3`, false}, {`a"b`, false}, {``, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			if got := explicitSource(tc.raw); got != tc.want {
				t.Errorf("explicitSource(%q) = %v, ожидалось %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestSourceQAMainFileRoutesByRawAndPreservesValue(t *testing.T) {
	base := t.TempDir()
	scope := filepath.Join(base, "resources")
	direct := sourceQAWrite(t, filepath.Join(base, "003.MP3"))
	automatic := sourceQAWrite(t, filepath.Join(scope, "prefix003.mp3suffix.MP3"))
	context := sourceQAContext(base)
	f := sourceQAFinder(base, scope)
	file, issue := f.mainFile("audio", `"raw-is-not-the-path.wav"`, "003.MP3", context)
	sourceQAFile(t, file, issue, direct, "audio/mpeg")
	file, issue = f.mainFile("audio", "003.MP3", "003.MP3", context)
	sourceQAFile(t, file, issue, automatic, "audio/mpeg")
	for _, tc := range []struct{ raw, value, name string }{
		{`"ignored"`, " 003.MP3", " 003.MP3"},
		{`"ignored"`, `literal\n.MP3`, `literal\n.MP3`},
		{`"ignored"`, "000000000000000000000003.MP3", "000000000000000000000003.MP3"},
		{"different raw", " 007.01 ", "prefix 007.01 suffix.mp3"},
		{"different raw", `literal\n`, `prefixliteral\nsuffix.mp3`},
		{"different raw", "000000000000000000000003", "prefix000000000000000000000003suffix.mp3"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			root := base
			if !explicitSource(tc.raw) {
				root = scope
			}
			want := sourceQAWrite(t, filepath.Join(root, tc.name))
			file, issue := f.mainFile("audio", tc.raw, tc.value, context)
			// На Windows обратная косая черта отделяет каталог от basename:
			// literal\n остаётся точной подстрокой, но в имени nsuffix.mp3 её нет.
			if os.IsPathSeparator('\\') && !explicitSource(tc.raw) && tc.value == `literal\n` {
				sourceQASelectionError(t, issue, CodeNotFound, context, "audio", tc.value, []string{scope}, nil)
				if issue.Reason != "Ресурс не найден" {
					t.Errorf("причина отсутствия: %q", issue.Reason)
				}
				return
			}
			sourceQAFile(t, file, issue, want, "audio/mpeg")
		})
	}
}
