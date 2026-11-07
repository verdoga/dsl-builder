package app

import (
	"fmt"
	"strings"
	"testing"
)

func TestDestinationArchiveNamePreservesValidSpelling(t *testing.T) {
	cases := []struct{ input, want string }{
		{"Unit1", "Unit1.zip"}, {"Unit1.zip", "Unit1.zip"},
		{"Unit1.ZIP", "Unit1.ZIP"}, {"Unit1.ZiP", "Unit1.ZiP"},
		{"Unit1.txt", "Unit1.txt.zip"}, {"Unit.zip.zip", "Unit.zip.zip"},
		{"Курс 日本語 🙂", "Курс 日本語 🙂.zip"},
		{"Résumé e\u0301", "Résumé e\u0301.zip"},
		{"a  b", "a  b.zip"}, {"a.b", "a.b.zip"},
		{"CONifer", "CONifer.zip"}, {"xCON", "xCON.zip"},
		{"COM0", "COM0.zip"}, {"COM10", "COM10.zip"},
		{"LPT0", "LPT0.zip"}, {"LPT10", "LPT10.zip"},
		{"COM⁴", "COM⁴.zip"}, {"LPT⁰", "LPT⁰.zip"},
		{"CONIN", "CONIN.zip"}, {"CONOUT", "CONOUT.zip"},
		{"valid!#$%&'()+,-;=@[]^_`{}~", "valid!#$%&'()+,-;=@[]^_`{}~.zip"},
		{strings.Repeat("Я", 1000), strings.Repeat("Я", 1000) + ".zip"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, problem := archiveName(tc.input)
			if problem != nil || got != tc.want {
				t.Fatalf("archiveName(%q) = %q, %v; want %q", tc.input, got, problem, tc.want)
			}
		})
	}
}

func TestDestinationArchiveNameRejectsWithoutRepair(t *testing.T) {
	invalid := []string{"", ".", "..", " ", "unit.", "unit ", " unit", ".unit",
		"unit.zip.", "unit.zip ", "/unit", `unit\part`, "unit/part", "C:unit", `C:\unit`,
		`\\server\share`, "a<b", "a>b", "a:b", `a"b`, "a|b", "a?b", "a*b"}
	for character := rune(0); character < 32; character++ {
		invalid = append(invalid, "a"+string(character)+"b")
	}
	for _, name := range invalid {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			got, problem := archiveName(name)
			if problem == nil || problem.Code != CodeArchiveName || problem.Reason == "" || got != "" {
				t.Fatalf("archiveName(%q) = %q, %#v; want CodeArchiveName and no repaired name", name, got, problem)
			}
		})
	}
}

func TestDestinationArchiveNameRejectsWindowsDevicesWithExtensions(t *testing.T) {
	devices := []string{"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$"}
	for _, prefix := range []string{"COM", "LPT"} {
		for _, number := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"} {
			devices = append(devices, prefix+number)
		}
	}
	for _, device := range devices {
		for _, name := range []string{device, strings.ToLower(device), device + ".zip", strings.ToLower(device) + ".txt", device + ".part.ZIP"} {
			t.Run(name, func(t *testing.T) {
				got, problem := archiveName(name)
				if got != "" || problem == nil || problem.Code != CodeArchiveName {
					t.Fatalf("archiveName(%q) = %q, %#v; reserved device accepted", name, got, problem)
				}
			})
		}
	}
}
