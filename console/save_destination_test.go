package console

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestConsoleSaveDestinationCompleteGroup(t *testing.T) {
	f := consoleSaveTestCreate(t, false)
	unchanged := consoleSaveTestUnchanged(t, f.target)
	before := f.op.Snapshot()
	if !before.Actions.ChooseDestination {
		t.Fatal("fixture cannot choose destination")
	}
	var out bytes.Buffer
	r := &consoleSaveTestReader{lines: []string{"1\n", `"` + filepath.Dir(f.target) + `"` + "\n", "1\n", "Результат модуля\n"}}
	r.before = func(index int) {
		if f.op.Snapshot().Target != nil {
			t.Fatalf("SetDestination before complete group at answer %d", index)
		}
	}
	d := consoleSaveTestDialog(f.op, r, &out)
	if err := d.chooseDestination(before); err != nil {
		t.Fatal(err)
	}
	after := f.op.Snapshot()
	if after.Target == nil || after.Target.Path != f.target || after.Target.Exists || after.Token == before.Token || after.Actions.Save {
		t.Fatalf("target or consent incorrect: %+v", after.Target)
	}
	unchanged()
	if strings.Index(out.String(), "Каталог") < 0 || strings.Index(out.String(), "Имя") < strings.Index(out.String(), "Каталог") {
		t.Fatalf("directory/name not separate: %s", out.String())
	}
}

func TestConsoleSaveDestinationCancellationAndReadFailure(t *testing.T) {
	fault := errors.New("save input broken")
	for _, tc := range []struct {
		name, input string
		cause       error
		cancelled   bool
	}{
		{"cancel-directory", "2\n", nil, false}, {"cancel-name", "1\nDIRECTORY\n2\n", nil, false},
		{"EOF-directory-menu", "", nil, true}, {"EOF-directory-value", "1\nfragment", nil, true},
		{"EOF-name-menu", "1\nDIRECTORY\n", nil, true}, {"EOF-name-value", "1\nDIRECTORY\n1\nfragment", nil, true},
		{"fault-directory-menu", "", fault, true}, {"fault-directory-value", "1\n", fault, true},
		{"fault-name-menu", "1\nDIRECTORY\n", fault, true}, {"fault-name-value", "1\nDIRECTORY\n1\n", fault, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := consoleSaveTestCreate(t, false)
			unchanged := consoleSaveTestUnchanged(t, f.target)
			input := strings.ReplaceAll(tc.input, "DIRECTORY", filepath.Dir(f.target))
			r := &consoleSaveTestReader{lines: strings.SplitAfter(input, "\n"), err: tc.cause}
			if input == "" {
				r.lines = nil
			}
			if len(r.lines) > 0 && r.lines[len(r.lines)-1] == "" {
				r.lines = r.lines[:len(r.lines)-1]
			}
			var out bytes.Buffer
			d := consoleSaveTestDialog(f.op, r, &out)
			err := d.chooseDestination(f.op.Snapshot())
			if tc.cancelled {
				if tc.cause != nil {
					if !errors.Is(err, tc.cause) || !consoleSaveTestHasCode(err, CodeInput) {
						t.Fatalf("input cause lost: %v", err)
					}
				} else if !errors.Is(err, io.EOF) {
					t.Fatalf("want EOF: %v", err)
				}
				if f.op.Snapshot().Actions.Cancel {
					t.Error("stream failure did not Cancel")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if f.op.Snapshot().Target != nil || f.op.Snapshot().Actions.Save {
				t.Error("partial target or consent published")
			}
			unchanged()
		})
	}
}

func TestConsoleSaveDestinationRawNamesBelongToApp(t *testing.T) {
	for _, name := range []string{`"quoted"`, " trailing ", "CON", `folder/name`, `folder\name`, "name.", "NUL.zip"} {
		t.Run(name, func(t *testing.T) {
			f := consoleSaveTestCreate(t, false)
			unchanged := consoleSaveTestUnchanged(t, f.target)
			var out bytes.Buffer
			d := consoleSaveTestDialog(f.op, strings.NewReader("1\n"+filepath.Dir(f.target)+"\n1\n"+name+"\n"), &out)
			if err := d.chooseDestination(f.op.Snapshot()); err != nil {
				t.Fatal(err)
			}
			if f.op.Snapshot().Target != nil || !strings.Contains(out.String(), app.CodeArchiveName) {
				t.Fatalf("raw owner rejection missing: %s", out.String())
			}
			unchanged()
		})
	}
}

func TestConsoleSaveDestinationDirectorySingleOuterPair(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`"Путь с пробелами"`, "Путь с пробелами"}, {`""quoted""`, `"quoted"`},
		{`C:\Users\Путь`, `C:\Users\Путь`}, {` "path" `, ` "path" `}, {`"a\nb"`, `a\nb`}, {`"unpaired`, `"unpaired`},
	} {
		if got := destinationDirectory(tc.raw); got != tc.want {
			t.Errorf("%q => %q; want %q", tc.raw, got, tc.want)
		}
	}
}

func TestConsoleSaveDestinationValueMenuOnlyAcceptsNumbers(t *testing.T) {
	f := consoleSaveTestCreate(t, false)
	unchanged := consoleSaveTestUnchanged(t, f.target)
	r := &consoleSaveTestReader{lines: []string{"yes\n", "0\n", "3\n", "\n", "1\n", filepath.Dir(f.target) + "\n", "2\n"}}
	var out bytes.Buffer
	if err := consoleSaveTestDialog(f.op, r, &out).chooseDestination(f.op.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if r.index != 7 || f.op.Snapshot().Target != nil || f.op.Snapshot().Actions.Save {
		t.Fatalf("invalid menu input changed group: consumed=%d target=%+v", r.index, f.op.Snapshot().Target)
	}
	unchanged()
}
