package resourceusage_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/resourceusage"
)

func task5ReportEquals(t *testing.T, got resourceusage.Report, used, unused []string) {
	t.Helper()
	if got.Total != len(used)+len(unused) || got.Used != len(used) || got.Unused != len(unused) {
		t.Errorf("counts = (%d, %d, %d), want (%d, %d, %d)",
			got.Total, got.Used, got.Unused, len(used)+len(unused), len(used), len(unused))
	}
	if got.Total != got.Used+got.Unused || got.Used != len(got.UsedPaths) || got.Unused != len(got.UnusedPaths) {
		t.Errorf("inconsistent report: %+v", got)
	}
	if !slices.Equal(got.UsedPaths, used) {
		t.Errorf("UsedPaths = %q, want %q", got.UsedPaths, used)
	}
	if !slices.Equal(got.UnusedPaths, unused) {
		t.Errorf("UnusedPaths = %q, want %q", got.UnusedPaths, unused)
	}
	seen := make(map[string]bool)
	for _, paths := range [][]string{got.UsedPaths, got.UnusedPaths} {
		for _, path := range paths {
			if seen[path] {
				t.Errorf("path appears more than once across report lists: %q", path)
			}
			seen[path] = true
		}
	}
}

func TestTask5ReportEmpty(t *testing.T) {
	task5ReportEquals(t, resourceusage.Report{}, nil, nil)
	var registry resourceusage.Registry
	task5ReportEquals(t, registry.Snapshot(), nil, nil)
	registry.Register(nil)
	registry.Register([]string{})
	registry.MarkUsed(nil)
	registry.MarkUsed([]string{})
	registry.Merge(nil)
	registry.Merge(&registry)
	task5ReportEquals(t, registry.Snapshot(), nil, nil)
}

func TestTask5ReportCountsAndPartition(t *testing.T) {
	const body = `C:\Unit 1\Audio\Track.mp3`
	const transcript = `C:\Unit 1\Audio\Track.txt`
	const sameNameElsewhere = `C:\Unit 2\Audio\Track.mp3`
	const unsupported = `C:\Unit 2\Audio\notes.bin`
	const external = `D:\External\Film.mp4`
	tests := []struct {
		name         string
		register     []string
		mark         []string
		used, unused []string
	}{
		{"all unused", []string{unsupported, body, body}, nil, nil, []string{body, unsupported}},
		{"all used without registration", nil, []string{external, body, external}, []string{body, external}, nil},
		{"mixed unique full paths", []string{sameNameElsewhere, transcript, body, unsupported, body},
			[]string{external, body, transcript, body}, []string{body, transcript, external}, []string{sameNameElsewhere, unsupported}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var registry resourceusage.Registry
			registry.Register(tt.register)
			registry.MarkUsed(tt.mark)
			registry.Register(tt.register)
			registry.MarkUsed(tt.mark)
			task5ReportEquals(t, registry.Snapshot(), tt.used, tt.unused)
		})
	}
}

func TestTask5ReportMergeIsUnion(t *testing.T) {
	const a = `C:\A\same.mp3`
	const b = `C:\B\same.mp3`
	const c = `C:\C\same.mp3`
	const d = `C:\D\same.mp3`
	const e = `D:\External\same.mp3`
	var first, second resourceusage.Registry
	first.Register([]string{a, b, c})
	first.MarkUsed([]string{a})
	second.Register([]string{a, b, d})
	second.MarkUsed([]string{b, e})
	before := first.Snapshot()
	first.Merge(&second)
	first.Merge(&second)
	first.Merge(&first)
	first.Merge(nil)
	task5ReportEquals(t, first.Snapshot(), []string{a, b, e}, []string{c, d})
	task5ReportEquals(t, second.Snapshot(), []string{b, e}, []string{a, d})
	task5ReportEquals(t, before, []string{a}, []string{b, c})
	second.MarkUsed([]string{d})
	second.Register([]string{`E:\Later\other.bin`})
	task5ReportEquals(t, first.Snapshot(), []string{a, b, e}, []string{c, d})
}

func TestTask5ReportGoLexicographicOrder(t *testing.T) {
	longPath := `\\?\C:\` + strings.Repeat(`Каталог с пробелом\`, 24) + `Ресурс.mp3`
	// Ожидание задано явно по порядку полных строк Go, без сортировки oracle.
	ordered := []string{
		`C:\Media 1\0.mp3`, `C:\Media 1\A.mp3`, `C:\Media 1\Z.mp3`,
		`C:\Media 1\a.mp3`, "C:\\Media 1\\e\u0301.mp3", `C:\Media 1\é.mp3`,
		`C:\Media 1\Ё.mp3`, `C:\Media 1\А.mp3`, `C:\Media 1\Я.mp3`,
		`C:\Media 1\а.mp3`, `C:\Media 1\ё.mp3`, `C:\Media 1\😀.mp3`,
		`C:\Media 2\A.mp3`, `C:\media 1\B.mp3`, longPath,
	}
	for _, mark := range []bool{false, true} {
		name := "unused"
		if mark {
			name = "used"
		}
		t.Run(name, func(t *testing.T) {
			var registry resourceusage.Registry
			for i := len(ordered) - 1; i >= 0; i-- {
				registry.Register([]string{ordered[i]})
				if mark {
					registry.MarkUsed([]string{ordered[i]})
				}
			}
			for i := 0; i < 32; i++ {
				if mark {
					task5ReportEquals(t, registry.Snapshot(), ordered, nil)
				} else {
					task5ReportEquals(t, registry.Snapshot(), nil, ordered)
				}
			}
		})
	}
}

func TestTask5ReportOwnsItsSlices(t *testing.T) {
	used := []string{`C:\Used\a.mp3`, `C:\Used\b.mp3`}
	unused := []string{`C:\Unused\a.bin`, `C:\Unused\b.bin`}
	var registry resourceusage.Registry
	registry.Register(unused)
	registry.MarkUsed(used)
	first, second := registry.Snapshot(), registry.Snapshot()
	first.UsedPaths[0] = `D:\Changed\used.mp3`
	if !slices.Equal(first.UnusedPaths, unused) {
		t.Error("UsedPaths mutation changed UnusedPaths in the same report")
	}
	first.UnusedPaths[0] = `D:\Changed\unused.bin`
	first.UsedPaths = append(first.UsedPaths, `D:\Changed\appended.mp3`)
	first.UnusedPaths = append(first.UnusedPaths, `D:\Changed\appended.bin`)
	first.Total, first.Used, first.Unused = -1, -2, -3
	task5ReportEquals(t, second, used, unused)
	task5ReportEquals(t, registry.Snapshot(), used, unused)
	second.UsedPaths[1] = `D:\Changed\second.mp3`
	second.UnusedPaths[1] = `D:\Changed\second.bin`
	task5ReportEquals(t, registry.Snapshot(), used, unused)
	historical := registry.Snapshot()
	registry.MarkUsed([]string{unused[0]})
	registry.Register([]string{`E:\Later\new.bin`})
	registry.MarkUsed([]string{`E:\Later\new.mp3`})
	task5ReportEquals(t, historical, used, unused)
	task5ReportEquals(t, registry.Snapshot(), []string{unused[0], used[0], used[1], `E:\Later\new.mp3`},
		[]string{unused[1], `E:\Later\new.bin`})
}

func TestTask5ReportDoesNotDependOnRegistrationOrMergeOrder(t *testing.T) {
	paths := []string{`C:\A\one.mp3`, `C:\B\two.bin`, `D:\External\three.txt`, `E:\Z\four.mp4`}
	for _, order := range [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}, {2, 0, 3, 1}} {
		var direct, merged resourceusage.Registry
		for _, i := range order {
			direct.Register([]string{paths[i]})
			var lesson resourceusage.Registry
			lesson.Register([]string{paths[i], paths[0]})
			if i%2 == 0 {
				direct.MarkUsed([]string{paths[i]})
				lesson.MarkUsed([]string{paths[i]})
			}
			merged.Merge(&lesson)
		}
		for i := 0; i < 32; i++ {
			task5ReportEquals(t, direct.Snapshot(), []string{paths[0], paths[2]}, []string{paths[1], paths[3]})
			task5ReportEquals(t, merged.Snapshot(), []string{paths[0], paths[2]}, []string{paths[1], paths[3]})
		}
	}
}
