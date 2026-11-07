package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/preview"
)

func integrationTestSaveOperation(t *testing.T, exists bool) (string, *app.Operation, *app.SavePreview) {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "json")
	integrationTestSource(t, dir, "SAVE41", "Урок", "SecretBody41", nil, nil)
	op := integrationTestCreateOperation(t, base, dir, "Курс", "")
	path := filepath.Join(base, "target.zip")
	if exists {
		if err := os.WriteFile(path, []byte("old target bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	integrationTestIssues(t, op.SetDestination(op.Snapshot().Token, base, "target.zip"))
	p, issues := op.ReviewSave(op.Snapshot().Token)
	integrationTestIssues(t, issues)
	if p == nil {
		t.Fatal("no real SavePreview")
	}
	return base, op, p
}

func TestConsoleIntegrationFullSavePreviewBeforeFirstConsent(t *testing.T) {
	base, op, p := integrationTestSaveOperation(t, false)
	composition, err := preview.Composition(p.Snapshot.Catalog, p.Snapshot.Module, p.Snapshot.NewFileNames)
	if err != nil {
		t.Fatal(err)
	}
	driver := &integrationTestDriver{t: t, steps: []integrationTestStep{{literal: "2", check: func(s string) {
		integrationTestContains(t, s, composition, preview.Resources(p.Snapshot.Resources), p.Destination.Path)
		if bytes.Contains([]byte(s), []byte("SecretBody41")) {
			t.Fatal("Node.Data leaked into preview")
		}
	}}}}
	d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
	if err := d.showSavePreview(p); err != nil {
		t.Fatal(err)
	}
	if _, err := d.confirmSaving(p); err != nil {
		t.Fatal(err)
	}
	integrationTestNoFile(t, p.Destination.Path)
	if op.Snapshot().Actions.Save {
		t.Fatal("rejected consent authorized Save")
	}
}

func TestConsoleIntegrationSaveRefusalEOFAndFaultNeverWrite(t *testing.T) {
	for _, scenario := range []string{"reject", "overwrite-reject", "eof", "overwrite-eof", "partial", "input-error", "output-error", "short-write"} {
		t.Run(scenario, func(t *testing.T) {
			exists := scenario == "overwrite-reject" || scenario == "overwrite-eof"
			base, op, p := integrationTestSaveOperation(t, exists)
			var before []byte
			if exists {
				before = integrationTestBytes(t, p.Destination.Path)
			}
			marker := errors.New("task41 refusal stream")
			driver := &integrationTestDriver{t: t}
			switch scenario {
			case "reject":
				driver.steps = []integrationTestStep{{literal: "2"}}
			case "overwrite-reject":
				driver.steps = []integrationTestStep{{literal: "1"}, {literal: "2", check: func(s string) { integrationTestContains(t, s, p.Destination.Path, "Перезапис") }}}
			case "overwrite-eof":
				driver.steps = []integrationTestStep{{literal: "1"}}
			case "partial":
				driver.final = []byte("1")
			case "input-error":
				driver.failure = marker
			}
			var output io.Writer = &driver.output
			if scenario == "output-error" {
				output = integrationTestBrokenWriter{cause: marker}
			}
			if scenario == "short-write" {
				output = integrationTestBrokenWriter{short: true}
			}
			d := dialog{input: bufio.NewReader(driver), output: output, baseDir: base, operation: op}
			_, err := d.confirmSaving(p)
			if scenario == "input-error" || scenario == "output-error" {
				if !errors.Is(err, marker) {
					t.Fatalf("cause: %v", err)
				}
			}
			if scenario == "short-write" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short write: %v", err)
			}
			if op.Snapshot().Actions.Save {
				t.Fatal("failure authorized positive confirmation")
			}
			if exists {
				if !bytes.Equal(before, integrationTestBytes(t, p.Destination.Path)) {
					t.Fatal("refusal overwrote existing target")
				}
			} else {
				integrationTestNoFile(t, p.Destination.Path)
			}
		})
	}
}

func TestConsoleIntegrationTargetRaceDoesNotAutomaticallyRetrySave(t *testing.T) {
	base, op, p := integrationTestSaveOperation(t, false)
	driver := &integrationTestDriver{t: t, steps: []integrationTestStep{{literal: "1", before: func() {
		if err := os.WriteFile(p.Destination.Path, []byte("other writer"), 0600); err != nil {
			t.Fatal(err)
		}
	}}}}
	d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
	if _, err := d.confirmSaving(p); err != nil {
		t.Fatal(err)
	}
	if string(integrationTestBytes(t, p.Destination.Path)) != "other writer" {
		t.Fatal("target race overwritten or automatic retry")
	}
	if op.Snapshot().Actions.Save {
		t.Fatal("race left consent reusable")
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "json" && entry.Name() != "target.zip" {
			t.Fatalf("unexpected transaction file %s", entry.Name())
		}
	}
}
