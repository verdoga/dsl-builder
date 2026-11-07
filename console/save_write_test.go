package console

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/moduleio"
)

func TestConsoleSaveWriteOnceRejectsTargetChangeWithoutAutomaticRetry(t *testing.T) {
	f := consoleSaveTestCreate(t, false)
	value := consoleSaveTestPreview(t, f)
	consoleSaveTestIssues(t, f.op.ConfirmSave(value.Token, true, false))
	if !f.op.Snapshot().Actions.Save {
		t.Fatal("fixture not confirmed")
	}
	if err := os.WriteFile(f.target, []byte("external replacement before Save"), 0644); err != nil {
		t.Fatal(err)
	}
	unchanged := consoleSaveTestUnchanged(t, f.target)
	var out bytes.Buffer
	r := &consoleSaveTestReader{lines: []string{"1\n", "1\n"}}
	d := consoleSaveTestDialog(f.op, r, &out)
	got, err := d.writeOnce()
	if err != nil || got != routePreview {
		t.Fatalf("route=%v err=%v", got, err)
	}
	if r.reads != 0 || f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil || !strings.Contains(out.String(), app.CodeTargetChanged) {
		t.Fatalf("unexpected retry/overwrite: %s", out.String())
	}
	unchanged()
	out.Reset()
	got, err = d.writeOnce()
	if err != nil || got != routePreview {
		t.Fatalf("second route=%v err=%v", got, err)
	}
	if !strings.Contains(out.String(), app.CodeUnconfirmed) || r.reads != 0 {
		t.Fatalf("second attempt bypassed NEW ReviewSave: %s", out.String())
	}
	unchanged()
}

func TestConsoleSaveWriteOnceWithNoConsentDoesNotWrite(t *testing.T) {
	f := consoleSaveTestCreate(t, true)
	consoleSaveTestTarget(t, f)
	unchanged := consoleSaveTestUnchanged(t, f.target)
	var out bytes.Buffer
	r := &consoleSaveTestReader{lines: []string{"1\n", "1\n"}}
	got, err := consoleSaveTestDialog(f.op, r, &out).writeOnce()
	if err != nil || got != routePreview || r.reads != 0 || f.op.Snapshot().WriteResult != nil {
		t.Fatalf("unconfirmed save continued route=%v err=%v", got, err)
	}
	if !strings.Contains(out.String(), app.CodeUnconfirmed) {
		t.Errorf("owner reason missing: %s", out.String())
	}
	unchanged()
}

func TestConsoleSaveManifestOnlyWritePreservesOriginalLessonBytes(t *testing.T) {
	f := consoleSaveTestUpdate(t, "", "")
	consoleSaveTestIssues(t, f.op.PrepareRevision(f.op.Snapshot().Token))
	old := f.old.OriginalBytes()
	if len(old) == 0 {
		t.Fatal("not a loaded lesson")
	}
	edit, issues := f.op.ProposeCourseTitle(f.op.Snapshot().Token, "Новый курс без правки урока")
	consoleSaveTestIssues(t, issues)
	if edit == nil {
		t.Fatal("no real edit")
	}
	consoleSaveTestIssues(t, f.op.AcceptEdit(edit.Token))
	s := f.op.Snapshot()
	if s.NoChanges || !s.Actions.ReviewSave || !s.RevisionReady {
		t.Fatalf("fixture not ready: %+v", s.Actions)
	}
	var out bytes.Buffer
	got, err := consoleSaveTestDialog(f.op, strings.NewReader("1\n1\n2\n"), &out).reviewSaving(s)
	if err != nil || got != routeExit {
		t.Fatalf("route=%v err=%v; %s", got, err, out.String())
	}
	consoleSaveTestIssues(t, moduleio.Verify(f.target))
	loaded, issues := moduleio.Read(f.target)
	consoleSaveTestIssues(t, issues)
	if len(loaded.Files) != 1 || !bytes.Equal(old, loaded.Files[0].OriginalBytes()) || !bytes.Equal(old, f.old.OriginalBytes()) {
		t.Error("unchanged lesson was reserialized/mutated")
	}
	if loaded.Manifest.Course.Title != "Новый курс без правки урока" {
		t.Error("accepted catalog edit missing")
	}
}
