package resourcefind

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
)

func TestSelectionPreservesFilesAndCompanionOrder(t *testing.T) {
	root := t.TempDir()
	var tree filetree.Resolver
	body := File{Entry: selectionTestEntry(t, &tree, root, "Video/Клип 01.mp4"), MIMEType: "video/mp4"}
	script := File{Entry: selectionTestEntry(t, &tree, root, "Video/Клип 01.txt"), MIMEType: "text/plain"}
	extensions := []string{"vtt", "srt", "ttml", "dfxp", "ass", "ssa"}
	mimes := []string{"text/vtt", "application/x-subrip", "application/ttml+xml", "application/ttml+xml", "text/plain", "text/plain"}
	subtitles := make([]File, len(extensions))
	for i, extension := range extensions {
		subtitles[i] = File{Entry: selectionTestEntry(t, &tree, root, "Video/Клип 01."+extension), MIMEType: mimes[i]}
	}
	// Невыбранный сосед не должен добавляться в учёт сборкой Selection.
	selectionTestEntry(t, &tree, root, "Video/Клип 01.extra")
	wantSubtitles := slices.Clone(subtitles)
	got := selection(body, &script, subtitles)
	selectionTestFile(t, got.Body, body)
	if got.Script == nil {
		t.Fatal("выбранная расшифровка потеряна")
	}
	selectionTestFile(t, *got.Script, script)
	if !slices.Equal(got.Subtitles, wantSubtitles) {
		t.Errorf("Subtitles = %+v, ожидается исходный порядок %+v", got.Subtitles, wantSubtitles)
	}
	wantPaths := []string{body.Entry.IdentityPath, script.Entry.IdentityPath}
	for _, file := range subtitles {
		wantPaths = append(wantPaths, file.Entry.IdentityPath)
	}
	slices.SortFunc(wantPaths, filetree.ComparePaths)
	if !slices.Equal(got.AccountedPaths, wantPaths) {
		t.Errorf("AccountedPaths = %q, ожидается %q", got.AccountedPaths, wantPaths)
	}
	if !slices.Equal(subtitles, wantSubtitles) {
		t.Error("selection изменил порядок или значения входного среза")
	}
}

func TestSelectionDeduplicatesAccountingWithoutRemovingRoles(t *testing.T) {
	root := t.TempDir()
	var tree filetree.Resolver
	entry := selectionTestEntry(t, &tree, root, "target.bin")
	aliases := []string{"Клип.mp4", "Клип.txt", "Клип.vtt", "Клип.srt"}
	files := make([]File, len(aliases))
	for i, name := range aliases {
		path := filepath.Join(root, name)
		if err := os.Link(entry.Path, path); err != nil {
			t.Fatal(err)
		}
		alias, err := tree.Inspect(path)
		if err != nil {
			t.Fatal(err)
		}
		if alias.IdentityPath != entry.IdentityPath {
			t.Fatal("fixture: псевдонимы не получили общую идентичность")
		}
		files[i] = File{Entry: alias, MIMEType: []string{"video/mp4", "text/plain", "text/vtt", "application/x-subrip"}[i]}
	}
	got := selection(files[0], &files[1], files[2:])
	selectionTestFile(t, got.Body, files[0])
	if got.Script == nil || !slices.Equal(got.Subtitles, files[2:]) {
		t.Fatalf("повтор IdentityPath удалил роли: %+v", got)
	}
	selectionTestFile(t, *got.Script, files[1])
	if !slices.Equal(got.AccountedPaths, []string{entry.IdentityPath}) {
		t.Errorf("AccountedPaths = %q, ожидается только %q", got.AccountedPaths, entry.IdentityPath)
	}
}

func TestSelectionOwnsMutableResultsAcrossCalls(t *testing.T) {
	root := t.TempDir()
	var tree filetree.Resolver
	body := File{Entry: selectionTestEntry(t, &tree, root, "body.mp4"), MIMEType: "video/mp4"}
	script := File{Entry: selectionTestEntry(t, &tree, root, "body.txt"), MIMEType: "text/plain"}
	subtitle := File{Entry: selectionTestEntry(t, &tree, root, "body.vtt"), MIMEType: "text/vtt"}
	input := []File{subtitle}
	first := selection(body, &script, input)
	second := selection(body, &script, input)
	if first.Script == nil || second.Script == nil || len(first.Subtitles) != 1 || len(first.AccountedPaths) != 3 {
		t.Fatal("неполная Selection")
	}
	wantPaths := slices.Clone(second.AccountedPaths)
	first.Body.MIMEType = "изменённое тело"
	first.Script.MIMEType = "изменённая расшифровка"
	first.Subtitles[0].MIMEType = "изменённые субтитры"
	first.AccountedPaths[0] = "изменённый путь"
	selectionTestFile(t, script, File{Entry: script.Entry, MIMEType: "text/plain"})
	selectionTestFile(t, input[0], subtitle)
	selectionTestFile(t, second.Body, body)
	selectionTestFile(t, *second.Script, File{Entry: script.Entry, MIMEType: "text/plain"})
	selectionTestFile(t, second.Subtitles[0], subtitle)
	if !slices.Equal(second.AccountedPaths, wantPaths) {
		t.Error("изменение первого результата затронуло учёт второго")
	}
	script.MIMEType = "новый вход"
	input[0] = body
	selectionTestFile(t, *second.Script, File{Entry: script.Entry, MIMEType: "text/plain"})
	selectionTestFile(t, second.Subtitles[0], subtitle)
}

func TestSelectionAllowsAbsentCompanions(t *testing.T) {
	var tree filetree.Resolver
	body := File{Entry: selectionTestEntry(t, &tree, t.TempDir(), "картинка.PNG"), MIMEType: "image/png"}
	for _, subtitles := range [][]File{nil, {}} {
		got := selection(body, nil, subtitles)
		selectionTestFile(t, got.Body, body)
		if got.Script != nil || len(got.Subtitles) != 0 {
			t.Errorf("отсутствующие companions стали непустыми: %+v", got)
		}
		if !slices.Equal(got.AccountedPaths, []string{body.Entry.IdentityPath}) {
			t.Errorf("AccountedPaths = %q", got.AccountedPaths)
		}
	}
}

func TestAccountedPathsUsesIdentityOrderAndIncludesEveryExtension(t *testing.T) {
	root := t.TempDir()
	var tree filetree.Resolver
	z := selectionTestEntry(t, &tree, root, "z/Исходный.UNKNOWN")
	m := selectionTestEntry(t, &tree, root, "m/одинаковое.mp3")
	a := selectionTestEntry(t, &tree, root, "a/одинаковое.mp3")
	aliasPath := filepath.Join(root, "A-alias.mp4")
	if err := os.Link(z.Path, aliasPath); err != nil {
		t.Fatal(err)
	}
	alias, err := tree.Inspect(aliasPath)
	if err != nil {
		t.Fatal(err)
	}
	input := []filetree.Entry{alias, m, z, a, alias, m}
	before := slices.Clone(input)
	want := []string{a.IdentityPath, m.IdentityPath, z.IdentityPath}
	got := accountedPaths(input)
	if !slices.Equal(got, want) {
		t.Errorf("AccountedPaths = %q, ожидается порядок IdentityPath %q", got, want)
	}
	if !slices.Equal(input, before) {
		t.Error("accountedPaths изменил входные entries")
	}
	other := accountedPaths(input)
	if len(got) != 0 {
		got[0] = "испорчено вызывающим кодом"
	}
	if !slices.Equal(other, want) || !slices.Equal(accountedPaths(input), want) {
		t.Error("результаты разных вызовов разделяют изменяемое состояние")
	}
}

func TestAccountedPathsDoesNotReinspectFiles(t *testing.T) {
	var tree filetree.Resolver
	entry := selectionTestEntry(t, &tree, t.TempDir(), "Байты не читаются.bin")
	if err := os.Remove(entry.Path); err != nil {
		t.Fatal(err)
	}
	if got := accountedPaths([]filetree.Entry{entry, entry}); !slices.Equal(got, []string{entry.IdentityPath}) {
		t.Errorf("учёт готовых Entry обратился к файловой системе: %q", got)
	}
	file := File{Entry: entry, MIMEType: "application/octet-stream"}
	got := selection(file, nil, nil)
	selectionTestFile(t, got.Body, file)
	if !slices.Equal(got.AccountedPaths, []string{entry.IdentityPath}) {
		t.Errorf("selection потерял готовую идентичность: %q", got.AccountedPaths)
	}
}

func TestAccountedPathsAllowsEmptyInput(t *testing.T) {
	for _, input := range [][]filetree.Entry{nil, {}} {
		if got := accountedPaths(input); len(got) != 0 {
			t.Errorf("пустой вход дал %q", got)
		}
	}
}

func TestAccountedPathsKeepsDistinctCaseVariantsAndTieOrder(t *testing.T) {
	root := t.TempDir()
	var tree filetree.Resolver
	upper := selectionTestEntry(t, &tree, root, "Case.bin")
	lowerPath := filepath.Join(root, "case.bin")
	file, err := os.OpenFile(lowerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		t.Skip("файловая система не поддерживает разные объекты Case.bin и case.bin")
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	lower, issue := tree.Inspect(lowerPath)
	if issue != nil {
		t.Fatal(issue)
	}
	if os.SameFile(upper.Info, lower.Info) || upper.IdentityPath == lower.IdentityPath {
		t.Fatal("fixture: требуются разные реальные объекты")
	}
	want := []string{upper.IdentityPath, lower.IdentityPath}
	if got := accountedPaths([]filetree.Entry{lower, upper, lower}); !slices.Equal(got, want) {
		t.Errorf("разные файлы объединены или нарушен второй ключ сортировки: %q, ожидается %q", got, want)
	}
}

func TestSelectionRetainsSymlinkDiscoveryAddressAndTargetInfo(t *testing.T) {
	root := t.TempDir()
	var tree filetree.Resolver
	target := selectionTestEntry(t, &tree, root, "target.bin")
	path := filepath.Join(root, "Выбранное имя.MP4")
	if err := os.Symlink(target.Path, path); err != nil {
		t.Fatal(err)
	}
	alias, err := tree.Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	if alias.Path != path || alias.IdentityPath != target.IdentityPath || alias.Info.Name() != "target.bin" {
		t.Fatalf("fixture: адрес ссылки/сведения цели не различаются: %+v", alias)
	}
	body := File{Entry: alias, MIMEType: "video/mp4"}
	got := selection(body, nil, nil)
	selectionTestFile(t, got.Body, body)
	if !slices.Equal(got.AccountedPaths, []string{target.IdentityPath}) {
		t.Errorf("в учёт попало имя обращения вместо IdentityPath: %q", got.AccountedPaths)
	}
}

func selectionTestEntry(t *testing.T, tree *filetree.Resolver, root, name string) filetree.Entry {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("реальный маленький ресурс\x00\xff"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry, err := tree.Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func selectionTestFile(t *testing.T, got, want File) {
	t.Helper()
	if got != want {
		t.Errorf("File = %+v, ожидается точное сохранение %+v", got, want)
	}
}
