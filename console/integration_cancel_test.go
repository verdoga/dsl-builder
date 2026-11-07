package console

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsoleIntegrationInitialGroupEOFAtEveryQuestion(t *testing.T) {
	for _, mode := range []int{1, 2, 3} {
		values := []string{"missing-input", "Курс", ""}
		if mode != 1 {
			values = []string{"missing.zip", "missing.json"}
		}
		all := []integrationTestStep{{literal: string(rune('0' + mode))}}
		all = integrationTestAppendValue(all, values...)
		for cut := 0; cut < len(all); cut++ {
			for _, partial := range []bool{false, true} {
				name := strings.Join([]string{string(rune('0' + mode)), string(rune('A' + cut))}, "/")
				if partial {
					name += "/partial"
				}
				t.Run(name, func(t *testing.T) {
					base := t.TempDir()
					d := &integrationTestDriver{t: t, steps: append([]integrationTestStep(nil), all[:cut]...)}
					if partial {
						d.final = []byte("unfinished-without-enter")
					}
					if err := Run(d, &d.output, base, "v41", nil); err != nil {
						t.Fatalf("EOF: %v", err)
					}
					integrationTestNoFile(t, filepath.Join(base, "missing.zip"))
					if strings.Contains(d.output.String(), "not_found") {
						t.Fatal("initial incomplete group reached Analyze")
					}
				})
			}
		}
	}
}

func TestConsoleIntegrationInitialGroupExplicitCancel(t *testing.T) {
	for _, mode := range []int{1, 2, 3} {
		count := 3
		if mode != 1 {
			count = 2
		}
		for cancel := 0; cancel < count; cancel++ {
			t.Run(string(rune('0'+mode))+string(rune('A'+cancel)), func(t *testing.T) {
				steps := []integrationTestStep{{literal: string(rune('0' + mode))}}
				values := []string{"missing.zip", "Курс", ""}
				steps = integrationTestAppendValue(steps, values[:cancel]...)
				steps = append(steps, integrationTestNumber(2, "Отмен"), integrationTestMenu("главн.*меню"), integrationTestNumber(4, "Выход|Выйти"))
				d := integrationTestRun(t, t.TempDir(), steps)
				if strings.Contains(d.output.String(), "not_found") {
					t.Fatal("cancelled group was analyzed")
				}
			})
		}
	}
}

type integrationTestBrokenWriter struct {
	short bool
	cause error
}

func (w integrationTestBrokenWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, w.cause
}
func TestConsoleIntegrationPublicStreamFailures(t *testing.T) {
	marker := errors.New("task41 stream")
	for _, short := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "short"}[short], func(t *testing.T) {
			err := Run(strings.NewReader("1\n"), integrationTestBrokenWriter{short: short, cause: marker}, t.TempDir(), "v41", nil)
			want := marker
			if short {
				want = io.ErrShortWrite
			}
			if !errors.Is(err, want) {
				t.Fatalf("lost output cause: %v", err)
			}
		})
	}
	d := &integrationTestDriver{t: t, failure: marker}
	var output bytes.Buffer
	if err := Run(d, &output, t.TempDir(), "v41", nil); !errors.Is(err, marker) {
		t.Fatalf("lost input cause: %v", err)
	}
}
