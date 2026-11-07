package resourcefind

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
)

func TestSourceQAExplicitFileUsesDiscoveryDirectory(t *testing.T) {
	root := t.TempDir()
	actualJSON := sourceQAWrite(t, filepath.Join(root, "old DSL", "actual.json"))
	discovery := filepath.Join(root, "обнаружение с пробелами")
	discoveredJSON := filepath.Join(discovery, "Урок.json")
	if err := os.MkdirAll(discovery, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actualJSON, discoveredJSON); err != nil {
		t.Fatal(err)
	}
	want := sourceQAWrite(t, filepath.Join(discovery, "media", "Ёж 003.MP3"))
	sourceQAWrite(t, filepath.Join(filepath.Dir(actualJSON), "media", "Ёж 003.MP3"))
	tree := &filetree.Resolver{}
	entry, inspectErr := tree.JSONFile(discoveredJSON)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	context := sourceQAContext(discovery)
	f, _, issue := Prepare(tree, entry.Path, nil, context)
	if issue != nil {
		t.Fatal(issue)
	}
	file, issue := f.explicitFile("audio", filepath.Join("media", ".", "Ёж 003.MP3"), context)
	sourceQAFile(t, file, issue, want, "audio/mpeg")
	external := sourceQAWrite(t, filepath.Join(root, "внешние ресурсы", "picture.AVIF"))
	file, issue = f.explicitFile("image", external, context)
	sourceQAFile(t, file, issue, external, "image/avif")
}

func TestSourceQAExplicitFileDoesNotSubstituteResourceDirectoryOrFallback(t *testing.T) {
	base := t.TempDir()
	scope := filepath.Join(base, "media")
	sourceQAWrite(t, filepath.Join(scope, "missing.mp3"))
	sourceQAWrite(t, filepath.Join(scope, "prefixmissing.mp3suffix.mp3"))
	f := sourceQAFinder(base, scope)
	context := sourceQAContext(base)
	_, issue := f.mainFile("audio", `"missing.mp3"`, "missing.mp3", context)
	if issue == nil || issue.Code != filetree.CodeAccess || issue.Context.Path != filepath.Join(base, "missing.mp3") {
		t.Fatalf("явный путь подменён поиском: %#v", issue)
	}
	if !errors.Is(issue, os.ErrNotExist) {
		t.Fatalf("потеряна исходная причина: %#v", issue)
	}
	found := false
	for _, related := range issue.Related {
		if related == context {
			found = true
		}
	}
	if !found {
		t.Fatalf("потерян контекст JSON: %#v", issue)
	}
}

func TestSourceQAExplicitFileRejectsKindAndExtensionWithContext(t *testing.T) {
	base := t.TempDir()
	directory := filepath.Join(base, "каталог.mp3")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	wrongType := sourceQAWrite(t, filepath.Join(base, "video.MP4"))
	unknown := sourceQAWrite(t, filepath.Join(base, "track.unknown"))
	f := sourceQAFinder(base)
	context := sourceQAContext(base)
	for _, tc := range []struct{ path, code string }{
		{directory, CodeKind}, {wrongType, CodeExtension}, {unknown, CodeExtension},
	} {
		t.Run(tc.code+filepath.Base(tc.path), func(t *testing.T) {
			_, issue := f.explicitFile("audio", tc.path, context)
			sourceQASelectionError(t, issue, tc.code, context, "audio", tc.path, nil, []string{tc.path})
		})
	}
}

func TestSourceQAExplicitFilePreservesFiletreeFailures(t *testing.T) {
	base := t.TempDir()
	context := sourceQAContext(base)
	f := sourceQAFinder(base)
	for _, tc := range []struct{ value, code string }{
		{"", filetree.CodePath}, {"bad\x00.mp3", filetree.CodePath}, {"нет.mp3", filetree.CodeAccess},
	} {
		t.Run(tc.code+tc.value, func(t *testing.T) {
			path, expected := filetree.Resolve(base, tc.value)
			if expected == nil {
				_, expected = (&filetree.Resolver{}).Inspect(path)
			}
			_, issue := f.explicitFile("audio", tc.value, context)
			if issue == nil || issue.Code != tc.code || issue.Code != expected.Code || issue.Reason != expected.Reason || issue.Context != expected.Context {
				t.Fatalf("ошибка зависимости изменена: got %#v, want %#v", issue, expected)
			}
			if (issue.Cause == nil) != (expected.Cause == nil) {
				t.Fatal("наличие Cause изменено")
			}
			if tc.code == filetree.CodeAccess && !errors.Is(issue, os.ErrNotExist) {
				t.Fatal("Cause не сохраняет ErrNotExist")
			}
			found := false
			for _, related := range issue.Related {
				if related == context {
					found = true
				}
			}
			if !found {
				t.Fatalf("нет контекста обращения: %#v", issue)
			}
		})
	}
}

func TestSourceQAExplicitFileChecksAliasExtension(t *testing.T) {
	base := t.TempDir()
	target := sourceQAWrite(t, filepath.Join(base, "target.MP4"))
	alias := filepath.Join(base, "audio alias.MP3")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	f := sourceQAFinder(base)
	context := sourceQAContext(base)
	file, issue := f.explicitFile("audio", "audio alias.MP3", context)
	sourceQAFile(t, file, issue, alias, "audio/mpeg")
	if file.Entry.IdentityPath != target {
		t.Errorf("Entry цели: %#v", file.Entry)
	}
	_, issue = f.explicitFile("video", "audio alias.MP3", context)
	sourceQASelectionError(t, issue, CodeExtension, context, "video", "audio alias.MP3", nil, []string{alias})
}

func TestSourceQAExplicitFileReportsRealAccessDenial(t *testing.T) {
	base := t.TempDir()
	restricted := filepath.Join(base, "restricted")
	target := sourceQAWrite(t, filepath.Join(restricted, "track.mp3"))
	if err := os.Chmod(restricted, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(restricted, 0755); err != nil {
			t.Error(err)
		}
	})
	// На Windows chmod не отнимает доступ, а root на Unix обходит mode bits.
	// Такой процесс не способен доказать отказ данным способом; отсутствие
	// объекта проверяется отдельным тестом и не подменяет этот сценарий.
	_, probe := os.Stat(target)
	if !errors.Is(probe, os.ErrPermission) {
		t.Skipf("mode bits не создают отказ доступа в этом окружении: %v", probe)
	}
	context := sourceQAContext(base)
	_, issue := sourceQAFinder(base).explicitFile("audio", target, context)
	if issue == nil || issue.Code != filetree.CodeAccess || issue.Context.Path != target || !errors.Is(issue, os.ErrPermission) {
		t.Fatalf("отказ доступа потерян: %#v", issue)
	}
	found := false
	for _, related := range issue.Related {
		if related == context {
			found = true
		}
	}
	if !found {
		t.Fatalf("контекст JSON отсутствует: %#v", issue)
	}
}
