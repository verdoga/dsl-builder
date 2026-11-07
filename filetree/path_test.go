package filetree

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestPathResolveWindowsExplicitBase(t *testing.T) {
	longName := strings.Repeat(`\Длинное имя`, 40)
	tests := []struct{ name, base, path, want string }{
		{"relative", `C:\Вход\Урок`, `..\Ресурсы/Звук 01.MP3`, `C:\Вход\Ресурсы\Звук 01.MP3`},
		{"different explicit base", `D:\Другой JSON`, `Audio\a.wav`, `D:\Другой JSON\Audio\a.wav`},
		{"absolute ignores absent base", "", `D:/Медиа/./Подкаталог/../A.wav`, `D:\Медиа\A.wav`},
		{"absolute ignores relative base", `not-a-base`, `C:\Медиа\A.wav`, `C:\Медиа\A.wav`},
		{"drive root", "", `C:\`, `C:\`},
		{"UNC absolute", "", `\\Server\Share\Каталог\..\A.wav`, `\\Server\Share\A.wav`},
		{"UNC forward separators", "", `//Server/Share/Audio/A.wav`, `\\Server\Share\Audio\A.wav`},
		{"UNC base", `\\Server\Share\JSON`, `..\Audio\A.wav`, `\\Server\Share\Audio\A.wav`},
		{"root relative", `D:\Вход\Урок`, `\Audio\..\A.wav`, `D:\A.wav`},
		{"root relative slash", `D:\Вход\Урок`, `/Audio/A.wav`, `D:\Audio\A.wav`},
		{"root relative UNC volume", `\\Server\Share\JSON`, `\Audio\A.wav`, `\\Server\Share\Audio\A.wav`},
		{"drive relative", `C:\Вход\Урок`, `C:..\Audio\A.wav`, `C:\Вход\Audio\A.wav`},
		{"drive relative volume case", `C:\JSON`, `c:Audio\A.wav`, `C:\JSON\Audio\A.wav`},
		{"dot", `C:\Вход\Урок`, `.`, `C:\Вход\Урок`},
		{"spaces preserved", `C:\JSON`, ` Audio \ A.wav `, `C:\JSON\ Audio \ A.wav `},
		{"quotes preserved", `C:\JSON`, `"A.wav"`, `C:\JSON\"A.wav"`},
		{"long drive path", "", `C:\JSON` + longName + `\A.wav`, `C:\JSON` + longName + `\A.wav`},
		{"long UNC path", "", `\\Server\Share` + longName + `\A.wav`, `\\Server\Share` + longName + `\A.wav`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.base, tt.path)
			if err != nil || got != tt.want {
				t.Fatalf("Resolve(%q, %q) = %q, %v; want %q", tt.base, tt.path, got, err, tt.want)
			}
		})
	}
}

func TestPathResolveRejectsImplicitWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	tests := []struct{ name, base, path string }{
		{"empty", `C:\JSON`, ""},
		{"relative without base", "", `Audio\a.wav`},
		{"relative base", `JSON`, `Audio\a.wav`},
		{"drive relative base", `C:JSON`, `Audio\a.wav`},
		{"root relative base", `\JSON`, `Audio\a.wav`},
		{"root relative without base", "", `\Audio\a.wav`},
		{"drive relative without base", "", `C:Audio\a.wav`},
		{"different drive", `D:\JSON`, `C:Audio\a.wav`},
		{"drive relative with UNC base", `\\Server\Share\JSON`, `C:Audio\a.wav`},
		{"incomplete UNC path", "", `\\Server`},
		{"incomplete UNC base", `\\Server`, `Audio\a.wav`},
		{"invalid NUL", `C:\JSON`, "bad\x00name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.base, tt.path)
			if got != "" {
				t.Errorf("Resolve returned result %q on invalid path", got)
			}
			pathTestError(t, err)
		})
	}
}

func TestPathRelativeWindowsWithoutFilesystem(t *testing.T) {
	// Все адреса вымышлены: Relative вычисляет строки, не проверяет объекты.
	tests := []struct{ name, base, path, want string }{
		{"child with mixed separators", `C:\Нет такого каталога\JSON`, `C:/Нет такого каталога/JSON/Audio/A.wav`, `Audio/A.wav`},
		{"outside", `C:\JSON\Урок`, `C:\Audio\a.wav`, `../../Audio/a.wav`},
		{"same", `C:\JSON`, `C:\JSON`, `.`},
		{"clean dots", `C:\JSON`, `C:\JSON\Part\..\A.wav`, `A.wav`},
		{"case insensitive prefix", `C:\JSON`, `c:\json\Audio\A.wav`, `Audio/A.wav`},
		{"UNC", `\\Server\Share\JSON`, `\\Server\Share\Audio\A.wav`, `../Audio/A.wav`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Relative(tt.base, tt.path)
			if err != nil || got != tt.want {
				t.Fatalf("Relative(%q, %q) = %q, %v; want %q", tt.base, tt.path, got, err, tt.want)
			}
		})
	}
	for _, tt := range []struct{ name, base, path string }{
		{"different drives", `C:\JSON`, `D:\Audio\a.wav`},
		{"different UNC shares", `\\Server\One\JSON`, `\\Server\Two\Audio\a.wav`},
		{"drive and UNC", `C:\JSON`, `\\Server\Share\a.wav`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Relative(tt.base, tt.path)
			if got != "" {
				t.Errorf("Relative returned partial result %q", got)
			}
			pathTestError(t, err)
		})
	}
}

func TestPathComparePathsOrder(t *testing.T) {
	for _, tt := range []struct {
		name, left, right string
		want              int
	}{
		{"lowercase primary key", "a/Z", "B/a", -1},
		{"original tie breaker", "Audio/A.wav", "audio/a.wav", -1},
		{"both separator forms", `Audio\A.wav`, `Audio/A.wav`, 0},
		{"normalize before tie breaker", `Audio\A.wav`, `audio/a.wav`, -1},
		{"Unicode primary key", `Ёж/A`, `Язык/A`, 1},
		{"Unicode original tie breaker", `Аудио/A`, `аудио/A`, -1},
		{"empty", "", "a", -1},
		{"equal", "a/b", "a/b", 0},
		{"lexical dots not cleaned", "a/../z", "a/b", -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComparePaths(tt.left, tt.right); got != tt.want {
				t.Errorf("ComparePaths(%q, %q) = %d; want %d", tt.left, tt.right, got, tt.want)
			}
			if got := ComparePaths(tt.right, tt.left); got != -tt.want {
				t.Errorf("reverse ComparePaths = %d; want %d", got, -tt.want)
			}
		})
	}
}

func TestPathSortRelativeIndependent(t *testing.T) {
	entries := []Entry{
		{Path: `C:\JSON\b\A.json`, IdentityPath: `C:\objects\one`},
		{Path: `C:\JSON\a.json`, IdentityPath: `C:\objects\two`},
		{Path: `C:\JSON\A.json`, IdentityPath: `C:\objects\three`},
		{Path: `C:\JSON\a\Z.json`, IdentityPath: `C:\objects\four`},
		{Path: `C:\Zulu.json`, IdentityPath: `C:\objects\five`},
	}
	original := slices.Clone(entries)
	want := []Entry{entries[4], entries[2], entries[1], entries[3], entries[0]}
	got, err := SortRelative(`C:\JSON`, entries)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("SortRelative = %#v, %v; want %#v", got, err, want)
	}
	if !slices.Equal(entries, original) {
		t.Fatal("SortRelative changed input")
	}
	got[0].Path = "changed output"
	if !slices.Equal(entries, original) {
		t.Fatal("SortRelative output aliases input backing array")
	}
	entries[1].Path = "changed input"
	if got[2].Path != want[2].Path {
		t.Fatal("SortRelative input mutation changed output")
	}
}

func TestPathSortRelativeAtomicAndEmpty(t *testing.T) {
	original := []Entry{
		{Path: `C:\JSON\b.json`, IdentityPath: `C:\objects\one`},
		{Path: `C:\JSON\a.json`, IdentityPath: `C:\objects\two`},
		{Path: `C:\JSON\A.json`, IdentityPath: `C:\objects\three`},
	}
	for _, badIndex := range []int{0, 2} {
		t.Run(string(rune('0'+badIndex)), func(t *testing.T) {
			invalid := slices.Clone(original)
			invalid[badIndex].Path = `D:\other\bad.json`
			before := slices.Clone(invalid)
			partial, err := SortRelative(`C:\JSON`, invalid)
			if partial != nil {
				t.Errorf("SortRelative returned partial result %#v", partial)
			}
			pathTestError(t, err)
			if !slices.Equal(invalid, before) {
				t.Error("SortRelative changed input on error")
			}
		})
	}
	for _, empty := range [][]Entry{nil, {}} {
		got, err := SortRelative(`C:\JSON`, empty)
		if err != nil || len(got) != 0 {
			t.Errorf("SortRelative empty = %#v, %v", got, err)
		}
	}
}

func TestPathUniqueExactIdentityAndFirstOrder(t *testing.T) {
	// Готовые Entry моделируют результат общего Resolver, включая псевдонимы.
	entries := []Entry{
		{Path: `C:\aliases\wrong.txt`, IdentityPath: `C:\objects\Object`},
		{Path: `C:\aliases\right.json`, IdentityPath: `C:\objects\Object`},
		{Path: `C:\same-address`, IdentityPath: `C:\objects\object`},
		{Path: `C:\same-address`, IdentityPath: `C:\objects\Other`},
		{Path: `C:\last-alias`, IdentityPath: `C:\objects\object`},
	}
	original := slices.Clone(entries)
	want := []Entry{entries[0], entries[2], entries[3]}
	got := Unique(entries)
	if !slices.Equal(got, want) {
		t.Fatalf("Unique = %#v; want first entries %#v", got, want)
	}
	if !slices.Equal(entries, original) {
		t.Fatal("Unique changed input")
	}
	got[0].IdentityPath = "changed output"
	if !slices.Equal(entries, original) {
		t.Fatal("Unique output aliases input backing array")
	}
	entries[2].Path = "changed input"
	if got[1].Path != want[1].Path {
		t.Fatal("Unique input mutation changed output")
	}
	for _, empty := range [][]Entry{nil, {}} {
		if got := Unique(empty); len(got) != 0 {
			t.Errorf("Unique empty = %#v", got)
		}
	}
	filtered := Unique(original[1:2])
	if len(filtered) != 1 || filtered[0].Path != original[1].Path {
		t.Fatal("Unique did not preserve the alias selected before deduplication")
	}
}

func TestPathF001ResolveRejectsIncompleteUNC(t *testing.T) {
	for _, tt := range []struct{ name, base, path string }{
		{"path backslash", "", `\\Server`},
		{"base backslash", `\\Server`, `Audio\a.wav`},
		{"path backslash trailing", "", `\\sErVeR\`},
		{"base backslash trailing", `\\sErVeR\`, `Audio\a.wav`},
		{"path slash", "", `//Server`},
		{"base slash", `//Server`, `Audio/a.wav`},
		{"path slash trailing", "", `//sErVeR/`},
		{"base slash trailing", `//sErVeR/`, `Audio/a.wav`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.base, tt.path)
			if got != "" {
				t.Errorf("Resolve(%q, %q) returned %q for incomplete UNC", tt.base, tt.path, got)
			}
			pathTestError(t, err)
		})
	}
}

func TestPathF002RelativeUNCShareRoot(t *testing.T) {
	// Корень share — полный адрес даже без завершающего разделителя.
	for _, tt := range []struct{ name, base, path, want string }{
		{"child to root", `\\Server\Share\JSON`, `\\Server\Share`, `..`},
		{"child to root trailing", `\\Server\Share\JSON`, `\\Server\Share\`, `..`},
		{"root trailing to root", `\\Server\Share\`, `\\Server\Share`, `.`},
		{"root trailing to root trailing", `\\Server\Share\`, `\\Server\Share\`, `.`},
		{"root to root", `\\Server\Share`, `\\Server\Share`, `.`},
		{"root to root trailing", `\\Server\Share`, `\\Server\Share\`, `.`},
		{"slash child to root", `//Server/Share/JSON`, `//Server/Share`, `..`},
		{"slash root trailing", `//Server/Share/`, `//Server/Share/`, `.`},
		{"volume case child to root", `\\sErVeR\sHaRe\JSON`, `//SERVER/SHARE`, `..`},
		{"volume case root to root", `\\sErVeR\sHaRe\`, `//SERVER/SHARE`, `.`},
		{"root to child preserves case", `\\Server\Share`, `//sErVeR/sHaRe/Audio/Звук 01.WAV`, `Audio/Звук 01.WAV`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Relative(tt.base, tt.path)
			if err != nil || got != tt.want {
				t.Fatalf("Relative(%q, %q) = %q, %v; want %q", tt.base, tt.path, got, err, tt.want)
			}
		})
	}
}

func pathTestError(t *testing.T, err *diagnostic.Error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected path diagnostic")
	}
	if err.Code != CodePath {
		t.Errorf("diagnostic Code = %q; want %q", err.Code, CodePath)
	}
	if !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("diagnostic Reason is not Russian: %q", err.Reason)
	}
}
