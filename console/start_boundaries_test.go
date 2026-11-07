package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestStartC01CollectsAllPathsBeforeAnyArchiveAnalysis(t *testing.T) {
	for _, mode := range []string{"update", "add"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			archive := filepath.Join(base, "не ZIP.zip")
			if err := os.WriteFile(archive, []byte("это реальные исходные байты, не ZIP"), 0600); err != nil {
				t.Fatal(err)
			}
			source := startTestSource(t, base, "json/урок.json", "BA82", nil)
			startTestUnchanged(t, archive, source)
			for _, finish := range []string{"2\n", "1\n" + source} {
				var output bytes.Buffer
				d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers(archive) + finish)), output: &output, baseDir: base}
				start := d.startUpdate
				if mode == "add" {
					start = d.startAdd
				}
				op, err := start()
				if op != nil || finish == "2\n" && err != nil || finish != "2\n" && !errors.Is(err, io.EOF) {
					t.Fatalf("неполный ввод: op=%v err=%v", op, err)
				}
				if strings.Contains(output.String(), "moduleio.") || strings.Contains(output.String(), "ZIP-архива") {
					t.Fatalf("ZIP прочитан до завершения группы: %s", &output)
				}
			}
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers(archive, source))), output: &output, baseDir: base}
			start := d.startUpdate
			if mode == "add" {
				start = d.startAdd
			}
			op, err := start()
			if op == nil || err != nil || output.Len() == 0 || len(op.Snapshot().Blockers) != 0 {
				t.Fatalf("полная группа должна только создать Operation: %v %v", op, err)
			}
			d.operation = op
			if err := d.analyze(); err != nil {
				t.Fatal(err)
			}
			snapshot := op.Snapshot()
			if len(snapshot.Blockers) == 0 || !snapshot.Actions.Retry || snapshot.Module != nil {
				t.Fatalf("реальный ZIP не проверен Analyze: %+v", snapshot)
			}
		})
	}
}

func TestStartConstructorDiagnosticFailureIsFatalForEveryMode(t *testing.T) {
	for _, mode := range []string{"create", "update", "add"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			_, ownerIssues := app.NewUpdate(base, "bad\x00path", "second")
			if len(ownerIssues) == 0 {
				t.Fatal("нет ошибки fixture")
			}
			values := []string{"bad\x00path", "second"}
			if mode == "create" {
				values = []string{values[0], "Курс", ""}
			}
			fault := errors.New("поток сломался на diagnostic")
			var emitted bytes.Buffer
			writer := startTestWriteFunc(func(raw []byte) (int, error) {
				if strings.Contains(emitted.String()+string(raw), ownerIssues[0].Code) {
					return 0, fault
				}
				return emitted.Write(raw)
			})
			d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers(values...))), output: writer, baseDir: base}
			start := d.startCreate
			if mode == "update" {
				start = d.startUpdate
			} else if mode == "add" {
				start = d.startAdd
			}
			op, err := start()
			if op != nil || !errors.Is(err, fault) {
				t.Fatalf("failed PrintError превратился в штатный возврат: %v %v", op, err)
			}
		})
	}
}

func TestStartUpdateRetryResetsDecisionsAfterGenuineCatalogBlocker(t *testing.T) {
	base := t.TempDir()
	archive := startTestArchive(t, base)
	source := startTestSource(t, base, "json/урок.json", "BA82", func(value *model.Result) {
		title := "Новое название"
		value.Document.Metadata.Title = &title
	})
	startTestUnchanged(t, archive)
	op, issues := app.NewUpdate(base, archive, source)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	d := dialog{operation: op, output: io.Discard}
	if err := d.analyze(); err != nil {
		t.Fatal(err)
	}
	snapshot := op.Snapshot()
	if len(snapshot.Blockers) != 0 || snapshot.Plan == nil || len(snapshot.Plan.Fields) == 0 {
		t.Fatalf("реальный план обновления не создан: %+v", snapshot)
	}
	for index := range snapshot.Plan.Fields {
		if issues := op.DecideField(op.Snapshot().Token, index, true); len(issues) != 0 {
			t.Fatal(issues)
		}
	}
	if issues := op.PrepareRevision(op.Snapshot().Token); len(issues) != 0 {
		t.Fatal(issues)
	}
	edit, issues := op.ProposeSection(op.Snapshot().Token, "Новая секция")
	if edit == nil {
		t.Fatalf("fixture исправляемой блокировки: %v", issues)
	}
	if issues := op.AcceptEdit(edit.Token); len(issues) == 0 {
		t.Fatal("ожидалась реальная блокировка незавершённого распределения")
	}
	blocked := op.Snapshot()
	if !blocked.Actions.Retry || !blocked.Actions.EditCatalog || !blocked.RevisionReady {
		t.Fatalf("fixture не готов для Retry: %+v", blocked)
	}
	startTestSource(t, base, "json/урок.json", "BA82", func(value *model.Result) {
		title := "Название после исправления"
		value.Document.Metadata.Title = &title
	})
	startTestUnchanged(t, source)
	if err := d.retry(); err != nil {
		t.Fatal(err)
	}
	fresh := op.Snapshot()
	if len(fresh.Blockers) != 0 || fresh.RevisionReady || fresh.Catalog.Sectioned || !fresh.Actions.DecideFields || fresh.Actions.Save || fresh.Target == nil || fresh.Target.Path != archive {
		t.Fatalf("Retry сохранил старые решения/ревизию: %+v", fresh)
	}
	for _, answered := range fresh.FieldAnswered {
		if answered {
			t.Fatal("Retry сохранил решение поля")
		}
	}
	for _, field := range fresh.Plan.Fields {
		if field.After != nil && *field.After == "Название после исправления" {
			return
		}
	}
	t.Fatal("Retry не обновил предложение поля")
}

func TestStartFailureChoiceOutputFailureStopsWithoutReading(t *testing.T) {
	fault := errors.New("сломанное меню продолжения")
	reads := 0
	d := dialog{
		input: bufio.NewReader(startTestReadFunc(func([]byte) (int, error) {
			reads++
			return 0, io.EOF
		})),
		output: startTestWriteFunc(func([]byte) (int, error) { return 0, fault }),
	}
	if _, err := d.startFailureChoice(); !errors.Is(err, fault) || reads != 0 {
		t.Fatalf("после ошибки вывода продолжен ввод: err=%v reads=%d", err, reads)
	}
}

func TestStartUpdateDirectoryIsRejectedOnlyByAnalyzeOwner(t *testing.T) {
	base := t.TempDir()
	archive := startTestArchive(t, base)
	source := startTestSource(t, base, "json/урок.json", "BA82", nil)
	startTestUnchanged(t, archive, source)
	d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers(archive, filepath.Dir(source)))), output: io.Discard, baseDir: base}
	op, err := d.startUpdate()
	if op == nil || err != nil || len(op.Snapshot().Blockers) != 0 {
		t.Fatalf("console сам проверил вид пути: %v %v", op, err)
	}
	d.operation = op
	if err := d.analyze(); err != nil {
		t.Fatal(err)
	}
	snapshot := op.Snapshot()
	if snapshot.Module != nil || len(snapshot.Blockers) == 0 || !snapshot.Actions.Retry {
		t.Fatalf("Update принял каталог вместо одного JSON: %+v", snapshot)
	}
}

func TestStartRetryAfterRealArchiveReplacementRebuildsDecisions(t *testing.T) {
	base := t.TempDir()
	archive := startTestArchive(t, base)
	source := startTestSource(t, base, "json/урок.json", "BA82", func(value *model.Result) {
		title := "Предлагаемое название"
		value.Document.Metadata.Title = &title
	})
	op, issues := app.NewUpdate(base, archive, source)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	d := dialog{operation: op, output: io.Discard}
	if err := d.analyze(); err != nil {
		t.Fatal(err)
	}
	snapshot := op.Snapshot()
	if snapshot.Plan == nil || len(snapshot.Plan.Fields) == 0 {
		t.Fatal("нет реального предложения поля")
	}
	for index := range snapshot.Plan.Fields {
		if issues := op.DecideField(op.Snapshot().Token, index, true); len(issues) != 0 {
			t.Fatal(issues)
		}
	}
	if issues := op.PrepareRevision(op.Snapshot().Token); len(issues) != 0 {
		t.Fatal(issues)
	}
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	// ZIP с внешним хвостом остаётся читаемым, но размер наблюдаемой цели меняется.
	if err := os.WriteFile(archive, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if preview, issues := op.ReviewSave(op.Snapshot().Token); preview != nil || len(issues) == 0 || !op.Snapshot().Actions.Retry {
		t.Fatalf("подмена архива не потребовала Retry: preview=%v issues=%v", preview, issues)
	}
	startTestUnchanged(t, archive, source)
	if err := d.retry(); err != nil {
		t.Fatal(err)
	}
	fresh := op.Snapshot()
	if len(fresh.Blockers) != 0 || fresh.RevisionReady || !fresh.Actions.DecideFields || fresh.Actions.Save || fresh.WriteResult != nil {
		t.Fatalf("Retry после замены архива сохранил разрешения: %+v", fresh)
	}
}
