package console

import (
	"bytes"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestConsoleSaveResultProjectionReportsEveryRecoveryPathTruthfully(t *testing.T) {
	for _, tc := range []struct {
		name                string
		installed, issues   bool
		original, temporary string
	}{
		{"success", true, false, "", ""},
		{"installed-cleanup-error", true, true, "/Каталог/резерв исходника.zip", ""},
		{"installed-temporary-error", true, true, "/Каталог/резерв исходника.zip", "/Каталог/временный результат.zip"},
		{"failed-original-and-temp", false, true, "/Каталог/резерв исходника.zip", "/Каталог/частичный результат.zip"},
		{"rolled-back", false, true, "/Каталог/цель.zip", ""},
		{"failed-new-temp", false, true, "", "/Каталог/временный результат.zip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := consoleSaveTestCreate(t, false)
			s := f.op.Snapshot()
			result := safewrite.Result{TargetPath: "/Каталог/цель.zip", Installed: tc.installed, OriginalPath: tc.original, TemporaryPath: tc.temporary}
			s.WriteResult = &result
			var issues []*diagnostic.Error
			if tc.issues {
				issues = []*diagnostic.Error{{Code: "safewrite.finish", Reason: "Не завершена очистка", Context: diagnostic.Context{Path: result.TargetPath}}, {Code: "safewrite.cleanup", Reason: "Остался временный файл", Details: []diagnostic.Detail{{Label: "Сохранённые сведения", Value: "все причины"}}}}
			}
			var out bytes.Buffer
			d := consoleSaveTestDialog(f.op, strings.NewReader(""), &out)
			if err := d.showWriteResult(s, issues); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{result.TargetPath, result.OriginalPath, result.TemporaryPath} {
				if path != "" && !strings.Contains(out.String(), path) {
					t.Errorf("known path hidden: %s; output %s", path, out.String())
				}
			}
			lower := strings.ToLower(out.String())
			if tc.issues {
				if !strings.Contains(out.String(), "safewrite.finish") || !strings.Contains(out.String(), "safewrite.cleanup") || !strings.Contains(out.String(), "все причины") {
					t.Error("issues/details lost")
				}
				if strings.Contains(lower, "успешно сохран") || strings.Contains(lower, "не изменён") {
					t.Errorf("false success/unchanged claim: %s", out.String())
				}
			} else if !strings.Contains(lower, "успешно") {
				t.Errorf("full success not stated: %s", out.String())
			}
			if result != *s.WriteResult || f.op.Snapshot().WriteResult != nil {
				t.Error("projection changed real owner state")
			}
		})
	}
}
