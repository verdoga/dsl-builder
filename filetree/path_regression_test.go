package filetree

import (
	"strings"
	"testing"
)

func TestPathF004ResolveExtendedUNCRootRelative(t *testing.T) {
	// Том расширенного UNC включает сервер и share, а не только \\?\UNC.
	for _, tt := range []struct{ name, base, path, want string }{
		{"senior example", `\\?\UNC\Server\Share\JSON`, `\Audio\a.wav`, `\\?\UNC\Server\Share\Audio\a.wav`},
		{"base root", `\\?\UNC\Server\Share`, `\Audio\a.wav`, `\\?\UNC\Server\Share\Audio\a.wav`},
		{"base root trailing", `\\?\UNC\Server\Share\`, `\Audio\a.wav`, `\\?\UNC\Server\Share\Audio\a.wav`},
		{"base child trailing", `\\?\UNC\Server\Share\JSON\`, `\Audio\a.wav`, `\\?\UNC\Server\Share\Audio\a.wav`},
		{"mixed separators", `\\?\UNC\Server\Share\JSON`, `/Audio/Звук 01.WAV`, `\\?\UNC\Server\Share\Audio\Звук 01.WAV`},
		{"volume spelling", `\\?\UNC\sErVeR\sHaRe\JSON`, `\Audio\Звук 01.WAV`, `\\?\UNC\sErVeR\sHaRe\Audio\Звук 01.WAV`},
		{"clean relative dots", `\\?\UNC\Server\Share\JSON`, `\Audio\..\Кадры\A.PNG`, `\\?\UNC\Server\Share\Кадры\A.PNG`},
		{"ordinary UNC unchanged", `\\Server\Share\JSON`, `\Audio\a.wav`, `\\Server\Share\Audio\a.wav`},
		{"ordinary drive unchanged", `D:\JSON`, `\Audio\a.wav`, `D:\Audio\a.wav`},
		{"extended drive unchanged", `\\?\D:\JSON`, `\Audio\a.wav`, `\\?\D:\Audio\a.wav`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.base, tt.path)
			if err != nil || got != tt.want {
				t.Fatalf("Resolve(%q, %q) = %q, %v; want %q", tt.base, tt.path, got, err, tt.want)
			}
		})
	}
}

func TestPathF004ResolveExtendedUNCSpellingAndLength(t *testing.T) {
	longPart := strings.Repeat(`\Длинное имя`, 40)
	for _, tt := range []struct{ name, base, path, want string }{
		{"absolute without base", "", `\\?\UNC\sErVeR\sHaRe\Audio\Звук 01.WAV`, `\\?\UNC\sErVeR\sHaRe\Audio\Звук 01.WAV`},
		{"absolute ignores other volume", `D:\JSON`, `\\?\UNC\sErVeR\sHaRe\Audio\A.WAV`, `\\?\UNC\sErVeR\sHaRe\Audio\A.WAV`},
		{"relative child", `\\?\UNC\sErVeR\sHaRe\JSON`, `Audio\Звук 01.WAV`, `\\?\UNC\sErVeR\sHaRe\JSON\Audio\Звук 01.WAV`},
		{"relative parent", `\\?\UNC\sErVeR\sHaRe\JSON`, `..\Audio\Звук 01.WAV`, `\\?\UNC\sErVeR\sHaRe\Audio\Звук 01.WAV`},
		{"long absolute UNC", "", `\\?\UNC\sErVeR\sHaRe` + longPart + `\A.WAV`, `\\?\UNC\sErVeR\sHaRe` + longPart + `\A.WAV`},
		{"long relative UNC", `\\?\UNC\sErVeR\sHaRe\JSON`, `Audio` + longPart + `\A.WAV`, `\\?\UNC\sErVeR\sHaRe\JSON\Audio` + longPart + `\A.WAV`},
		{"long root relative UNC", `\\?\UNC\sErVeR\sHaRe\JSON`, `\Audio` + longPart + `\A.WAV`, `\\?\UNC\sErVeR\sHaRe\Audio` + longPart + `\A.WAV`},
		{"long extended drive", "", `\\?\D:\JSON` + longPart + `\A.WAV`, `\\?\D:\JSON` + longPart + `\A.WAV`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.base, tt.path)
			if err != nil || got != tt.want {
				t.Fatalf("Resolve(%q, %q) = %q, %v; want %q", tt.base, tt.path, got, err, tt.want)
			}
		})
	}
}

func TestPathF004RelativeRejectsDifferentExtendedUNCVolumes(t *testing.T) {
	for _, tt := range []struct{ name, base, path string }{
		{"different shares", `\\?\UNC\Server\shareOne\JSON`, `\\?\UNC\Server\shareTwo\Audio\a.wav`},
		{"different shares at root", `\\?\UNC\Server\shareOne\`, `\\?\UNC\Server\shareTwo`},
		{"share prefix is not volume match", `\\?\UNC\Server\Share\JSON`, `\\?\UNC\Server\ShareExtra\A.wav`},
		{"different servers", `\\?\UNC\ServerOne\Share\JSON`, `\\?\UNC\ServerTwo\Share\A.wav`},
		{"different shares mixed case", `\\?\UNC\sErVeR\sHaReOne\JSON`, `\\?\UNC\SERVER\SHARETwo\A.wav`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Relative(tt.base, tt.path)
			if got != "" {
				t.Errorf("Relative(%q, %q) returned partial result %q", tt.base, tt.path, got)
			}
			pathTestError(t, err)
			if err.Context.Path != tt.path {
				t.Errorf("diagnostic path = %q; want %q", err.Context.Path, tt.path)
			}
		})
	}
}

func TestPathF004RelativeExtendedUNCSameShare(t *testing.T) {
	// Адреса вымышленные: ни UNC-сервер, ни файлы для вычисления не нужны.
	longPart := strings.Repeat(`\Длинное имя`, 40)
	longRelative := strings.Repeat(`/Длинное имя`, 40)
	for _, tt := range []struct{ name, base, path, want string }{
		{"sibling", `\\?\UNC\Server\Share\JSON`, `\\?\UNC\Server\Share\Audio\a.wav`, `../Audio/a.wav`},
		{"child to root", `\\?\UNC\Server\Share\JSON`, `\\?\UNC\Server\Share`, `..`},
		{"child to root trailing", `\\?\UNC\Server\Share\JSON`, `\\?\UNC\Server\Share\`, `..`},
		{"root to root", `\\?\UNC\Server\Share`, `\\?\UNC\Server\Share`, `.`},
		{"root to root trailing", `\\?\UNC\Server\Share`, `\\?\UNC\Server\Share\`, `.`},
		{"root trailing to root", `\\?\UNC\Server\Share\`, `\\?\UNC\Server\Share`, `.`},
		{"root trailing to root trailing", `\\?\UNC\Server\Share\`, `\\?\UNC\Server\Share\`, `.`},
		{"root to child", `\\?\UNC\Server\Share`, `\\?\UNC\Server\Share\Audio\A.WAV`, `Audio/A.WAV`},
		{"root trailing to child", `\\?\UNC\Server\Share\`, `\\?\UNC\Server\Share\Audio\A.WAV`, `Audio/A.WAV`},
		{"case insensitive volume", `\\?\UNC\sErVeR\sHaRe\JSON`, `\\?\UNC\SERVER\SHARE\Audio\Звук 01.WAV`, `../Audio/Звук 01.WAV`},
		{"case insensitive shared directory", `\\?\UNC\Server\Share\JSON`, `\\?\UNC\server\share\json\AuDio\A.WAV`, `AuDio/A.WAV`},
		{"case insensitive root", `\\?\UNC\sErVeR\sHaRe\`, `\\?\UNC\SERVER\SHARE`, `.`},
		{"child trailing", `\\?\UNC\Server\Share\JSON\`, `\\?\UNC\Server\Share\JSON\Audio\`, `Audio`},
		{"long target", `\\?\UNC\sErVeR\sHaRe\JSON`, `\\?\UNC\SERVER\SHARE\JSON\Audio` + longPart + `\A.WAV`, `Audio` + longRelative + `/A.WAV`},
		{"long shared prefix", `\\?\UNC\sErVeR\sHaRe` + longPart, `\\?\UNC\SERVER\SHARE` + longPart + `\AuDio\A.WAV`, `AuDio/A.WAV`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Relative(tt.base, tt.path)
			if err != nil || got != tt.want {
				t.Fatalf("Relative(%q, %q) = %q, %v; want %q", tt.base, tt.path, got, err, tt.want)
			}
		})
	}
}
