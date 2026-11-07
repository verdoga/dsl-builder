package preview

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestResourcesPreparedCountersAndScope(t *testing.T) {
	// Intentionally different from slice lengths: preview must display the
	// prepared counters, rather than reconstructing the owner's accounting.
	report := resourceusage.Report{
		Total: 37, Used: 23, Unused: 14,
		UsedPaths:   []string{"/not-present/used.mp3"},
		UnusedPaths: []string{"/not-present/unused.vtt"},
	}
	for name, render := range map[string]func(resourceusage.Report) string{
		"summary": Resources, "unused": UnusedResources,
	} {
		t.Run(name, func(t *testing.T) {
			text := render(report)
			resourcesTestCounters(t, text, report)
			resourcesTestScope(t, text)
			resourcesTestSafeBlock(t, text)
		})
	}
}

func TestUnusedResourcesPreservesFullOrderAndInput(t *testing.T) {
	paths := []string{
		`Z:\Нет такого каталога\音楽\  Последний файл .mp3`,
		`C:\Нет такого каталога\Первый\одинаковое имя.txt`,
		`C:\Нет такого каталога\Другой\одинаковое имя.txt`,
		"/not-present/" + strings.Repeat("длинный путь λ ", 90) + "полный конец.vtt",
	}
	report := resourceusage.Report{
		Total: 6, Used: 2, Unused: len(paths),
		UsedPaths:   []string{"/not-present/used.mp3", "/not-present/used.vtt"},
		UnusedPaths: slices.Clone(paths),
	}
	usedBefore := slices.Clone(report.UsedPaths)
	unusedBefore := slices.Clone(report.UnusedPaths)
	summary := Resources(report)
	text := UnusedResources(report)
	if !strings.Contains(text, summary) {
		t.Errorf("unused block must include the Resources block:\n%s", text)
	}
	resourcesTestCounters(t, text, report)
	resourcesTestScope(t, text)
	resourcesTestSafeBlock(t, text)
	resourcesTestNumberedPaths(t, text, paths)
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "не будут") ||
		(!strings.Contains(lower, "встро") && !strings.Contains(lower, "включ")) ||
		!strings.Contains(lower, "исходник") {
		t.Errorf("missing prospective explanation about current sources:\n%s", text)
	}
	if report.Total != 6 || report.Used != 2 || report.Unused != 4 ||
		!slices.Equal(report.UsedPaths, usedBefore) || !slices.Equal(report.UnusedPaths, unusedBefore) {
		t.Fatalf("rendering changed the report: %+v", report)
	}
	if again := UnusedResources(report); again != text {
		t.Fatal("repeated rendering changed the result")
	}
}

func TestUnusedResourcesEmptyListIsExplicit(t *testing.T) {
	for _, paths := range [][]string{nil, {}} {
		report := resourceusage.Report{UnusedPaths: paths}
		text := UnusedResources(report)
		if !strings.Contains(text, "Неиспользованных файлов нет") {
			t.Errorf("missing contract phrase for an empty list:\n%s", text)
		}
		resourcesTestCounters(t, text, report)
		resourcesTestScope(t, text)
		resourcesTestSafeBlock(t, text)
		resourcesTestNumberedPaths(t, text, nil)
		if (report.UnusedPaths == nil) != (paths == nil) {
			t.Fatal("rendering changed nil/empty ownership")
		}
	}
}

func TestUnusedResourcesEscapesExternalControlCharacters(t *testing.T) {
	paths := []string{
		"/not-present/  Кириллица λ 音楽 \"кавычки\" .txt",
		"/not-present/строка\nвторая\r\t\x1b[31m\x00\x7f.vtt",
	}
	report := resourceusage.Report{Total: 2, Unused: 2, UnusedPaths: slices.Clone(paths)}
	text := UnusedResources(report)
	resourcesTestSafeBlock(t, text)
	resourcesTestNumberedPaths(t, text, paths)
	if !slices.Equal(report.UnusedPaths, paths) {
		t.Fatal("escaping changed original paths")
	}
}

func TestResourcesAcceptsRealRegistrySnapshot(t *testing.T) {
	var registry resourceusage.Registry
	registry.Register([]string{"/not-present/external.mp3", "/not-present/external.vtt", "/not-present/unused.bin"})
	registry.MarkUsed([]string{"/not-present/external.mp3", "/not-present/external.vtt"})
	report := registry.Snapshot()
	for _, text := range []string{Resources(report), UnusedResources(report)} {
		resourcesTestCounters(t, text, report)
		resourcesTestScope(t, text)
		resourcesTestSafeBlock(t, text)
	}
	resourcesTestNumberedPaths(t, UnusedResources(report), report.UnusedPaths)
	if after := registry.Snapshot(); after.Total != report.Total || after.Used != report.Used ||
		after.Unused != report.Unused || !slices.Equal(after.UsedPaths, report.UsedPaths) ||
		!slices.Equal(after.UnusedPaths, report.UnusedPaths) {
		t.Fatal("rendering changed the registry snapshot")
	}
}

func resourcesTestCounters(t *testing.T, text string, report resourceusage.Report) {
	t.Helper()
	// Labels may contain extra words or punctuation; layout is not a golden.
	pattern := regexp.MustCompile(`(?i)(всего|не[ \t]*использ\p{L}*|использ\p{L}*)[^0-9\n]*([0-9]+)`)
	found := make(map[string]int)
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		label := strings.ToLower(match[1])
		key := "used"
		if label == "всего" {
			key = "total"
		} else if strings.HasPrefix(label, "не") {
			key = "unused"
		}
		number, err := strconv.Atoi(match[2])
		if err != nil {
			t.Fatal(err)
		}
		found[key] = number
	}
	for key, want := range map[string]int{"total": report.Total, "used": report.Used, "unused": report.Unused} {
		if got, ok := found[key]; !ok || got != want {
			t.Errorf("%s counter: got %d (present %v), want %d:\n%s", key, got, ok, want, text)
		}
	}
}

func resourcesTestScope(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, stem := range []string{"текущ", "исходник", "сопутств"} {
		if !strings.Contains(lower, stem) {
			t.Errorf("resource scope is missing %q:\n%s", stem, text)
		}
	}
	if !strings.Contains(lower, "resourcedirs") &&
		(!strings.Contains(lower, "ресурсн") || !strings.Contains(lower, "каталог")) {
		t.Errorf("resource scope must include resource directories:\n%s", text)
	}
	if !strings.Contains(lower, "явн") && !strings.Contains(lower, "внешн") {
		t.Errorf("resource scope must include explicit external files:\n%s", text)
	}
}

func resourcesTestSafeBlock(t *testing.T, text string) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Fatal("output is not UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("raw control U+%04X in block", r)
		}
	}
	lower := strings.ToLower(text)
	for _, forbidden := range []string{"ваш выбор", "вернуться", "главное меню", "ошибка сборки", "запись запрещена"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("unexpected menu or blocking diagnostic %q:\n%s", forbidden, text)
		}
	}
}

func resourcesTestNumberedPaths(t *testing.T, text string, paths []string) {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^\s*([0-9]+)\s*[.):—-]\s*(.*)$`)
	items := pattern.FindAllStringSubmatch(text, -1)
	if len(items) != len(paths) {
		t.Fatalf("numbered items: got %d, want %d:\n%s", len(items), len(paths), text)
	}
	for i, path := range paths {
		if number, _ := strconv.Atoi(items[i][1]); number != i+1 {
			t.Errorf("item %d has number %d", i, number)
		}
		body := strings.TrimSpace(items[i][2])
		decoded, err := strconv.Unquote(body)
		if err != nil {
			// Ordinary paths may also be displayed literally; controlled strings
			// must be quoted and reversibly escaped to remain on one line.
			decoded = body
		}
		if decoded != path {
			t.Errorf("item %d: got %q, want complete path %q", i+1, decoded, path)
		}
	}
}
