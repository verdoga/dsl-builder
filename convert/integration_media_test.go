package convert_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func integrationPaths(t *testing.T, tree *filetree.Resolver, root string, names []string) []string {
	t.Helper()
	paths := make([]string, 0, len(names))
	for _, name := range names {
		entry, issue := tree.Inspect(filepath.Join(root, filepath.FromSlash(name)))
		if issue != nil {
			t.Fatal(issue)
		}
		paths = append(paths, entry.IdentityPath)
	}
	slices.Sort(paths)
	return paths
}

func integrationUsage(t *testing.T, usage *resourceusage.Registry, used, unused []string) {
	t.Helper()
	report := usage.Snapshot()
	if report.Total != len(used)+len(unused) || report.Used != len(used) || report.Unused != len(unused) ||
		!slices.Equal(report.UsedPaths, used) || !slices.Equal(report.UnusedPaths, unused) {
		t.Fatalf("Неверный учёт: %+v; использованы %v; не использованы %v", report, used, unused)
	}
	if len(report.UsedPaths) > 0 {
		report.UsedPaths[0] = "изменён снимок"
		if !slices.Equal(usage.Snapshot().UsedPaths, used) {
			t.Fatal("Snapshot разделяет изменяемую память с реестром")
		}
	}
}

func TestIntegrationSyntheticResourcesFragmentsAndIndependentCalls(t *testing.T) {
	root := integrationFixture(t)
	var tree filetree.Resolver
	input := integrationLoad(t, &tree, filepath.Join(root, "synthetic-composite.parsed.json"))
	want := integrationExpected(t, "synthetic-composite.expected.json")
	first, firstUsage := integrationBuild(t, input, &tree)
	second, secondUsage := integrationBuild(t, input, &tree)
	integrationEqual(t, first, want)
	integrationEqual(t, second, want)
	used := integrationPaths(t, &tree, root, []string{
		"resources/Tone 003.MP4", "resources/Tone 003.txt", "resources/Tone 003.vtt", "resources/Tone 003.srt", "external/карта.png",
	})
	unused := integrationPaths(t, &tree, root, []string{"resources/unused.bin", "resources/nested/Tone 003.vtt"})
	integrationUsage(t, firstUsage, used, unused)
	integrationUsage(t, secondUsage, used, unused)

	// Оба экземпляра медиа сохраняют все роли, хотя использованные пути общие.
	media := []*lesson.Node{&first.Document.Pages[0].Nodes[0].Nodes[0].Nodes[0], &first.Document.Pages[1].Nodes[0]}
	for _, node := range media {
		for _, datum := range node.Data {
			file, ok := datum.Value.(lesson.EmbeddedFile)
			if !ok {
				t.Fatalf("Встроенный файл имеет тип %T", datum.Value)
			}
			decoded, err := base64.StdEncoding.DecodeString(file.Content)
			if err != nil || !bytes.Equal(decoded, integrationRead(t, filepath.Join(root, "resources", file.FileName))) {
				t.Fatalf("Потеря исходных байтов %s: %v", file.FileName, err)
			}
		}
	}
	// Изменение первого include не меняет второе в том же результате.
	media[0].Data[0].Value = lesson.EmbeddedFile{FileName: "изменён"}
	if !bytes.Equal(integrationJSON(t, media[1]), integrationJSON(t, &want.Document.Pages[1].Nodes[0])) {
		t.Fatal("Два включения разделяют данные медиа")
	}
	*first.Document.Subtitle = "изменён"
	*first.Document.Pages[0].Title = "изменена"
	*first.Document.Pages[0].Nodes[0].Flags.Speaking = false
	question := &first.Document.Pages[0].Nodes[0].Nodes[1].Nodes[1]
	question.Data[0].Answers[0] = "изменён"
	*question.Data[0].Role = lesson.RoleTitle
	*question.Flags.Checkable = false
	firstUsage.MarkUsed(unused)
	integrationEqual(t, second, want)
	integrationUsage(t, secondUsage, used, unused)
	third, thirdUsage := integrationBuild(t, input, &tree)
	integrationEqual(t, third, want)
	integrationUsage(t, thirdUsage, used, unused)
	var merged resourceusage.Registry
	merged.Merge(secondUsage)
	merged.Merge(thirdUsage)
	integrationUsage(t, &merged, used, unused)
	for _, name := range []string{"Tone 003.MP4", "Tone 003.txt", "Tone 003.vtt", "Tone 003.srt"} {
		if !bytes.Equal(integrationRead(t, filepath.Join(root, "resources", name)), integrationRead(t, filepath.Join("testdata", "integration", "resources", name))) {
			t.Errorf("Изменён исходный ресурс %s", name)
		}
	}
}

func TestIntegrationNoMediaStillInventoriesDeclaredDirectories(t *testing.T) {
	root := integrationFixture(t)
	var tree filetree.Resolver
	input := integrationLoad(t, &tree, filepath.Join(root, "synthetic-no-media.parsed.json"))
	value, usage := integrationBuild(t, input, &tree)
	if len(value.Document.Pages) != 1 || len(value.Document.Pages[0].Nodes) != 1 || value.Document.Pages[0].Nodes[0].Type != lesson.NodeText {
		t.Fatal("Простой текст потерян при подготовке ресурсной области")
	}
	unused := integrationPaths(t, &tree, root, []string{
		"resources/Tone 003.MP4", "resources/Tone 003.txt", "resources/Tone 003.vtt", "resources/Tone 003.srt", "resources/unused.bin", "resources/nested/Tone 003.vtt",
	})
	integrationUsage(t, usage, nil, unused)
	if _, err := os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Fatalf("Каталог missing должен отсутствовать: %v", err)
	}
	broken := integrationLoad(t, &tree, filepath.Join(root, "synthetic-no-media-missing-dir.parsed.json"))
	got, registry, issues := integrationFailure(t, broken, &tree, integrationAt())
	if got != nil || registry != nil || len(issues) != 1 || issues[0].Code != filetree.CodeAccess || issues[0].Context.Path != filepath.Join(root, "missing") || issues[0].Cause == nil {
		t.Fatalf("Объявленный отсутствующий каталог без media не проверен: %v", issues)
	}
	if !slices.ContainsFunc(issues[0].Related, func(c diagnostic.Context) bool { return c.Path == broken.Path && c.LessonID == "Mix-A9" }) {
		t.Fatal("Ошибка каталога потеряла контекст JSON")
	}
}
