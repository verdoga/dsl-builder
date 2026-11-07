package app_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourcefind"
)

func TestIntegrationCreateResourcesAreUniqueAndExactAcrossLessons(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	files := map[string][]byte{
		"nested/Clip.MP3": {0, 1, 2, 255, 0, 42}, "nested/Clip.txt": []byte("Файловая расшифровка\r\n"),
		"nested/Clip.vtt": []byte("WEBVTT\nexact bytes\n"), "nested/Clip.srt": []byte("1\r\nsubtitle\r\n"), "unused.bin": {9, 8, 7},
	}
	for name, raw := range files {
		integrationWrite(t, filepath.Join(base, "media", filepath.FromSlash(name)), raw)
	}
	sourceRaw := map[string][]byte{}
	for _, id := range []string{"MEDIA1", "MEDIA2"} {
		path := filepath.Join(source, id+".json")
		integrationWriteInput(t, path, integrationMediaInput(id))
		sourceRaw[path] = integrationRead(t, path)
	}
	op := integrationCreate(t, base, source)
	snapshot := op.Snapshot()
	if snapshot.Resources.Total != 5 || snapshot.Resources.Used != 4 || snapshot.Resources.Unused != 1 || len(snapshot.Resources.UsedPaths) != 4 {
		t.Fatalf("resources counted per lesson: %+v", snapshot.Resources)
	}
	if len(snapshot.Resources.UnusedPaths) != 1 || snapshot.Resources.UnusedPaths[0] != filepath.Join(base, "media", "unused.bin") {
		t.Fatalf("incorrect unused resources: %+v", snapshot.Resources)
	}
	snapshot.Resources.UsedPaths[0] = "forged"
	snapshot.Resources.UnusedPaths[0] = "forged"
	if op.Snapshot().Resources.UsedPaths[0] == "forged" || op.Snapshot().Resources.UnusedPaths[0] == "forged" {
		t.Fatal("registry snapshot mutable")
	}
	integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "media"))
	value := integrationSave(t, op, false)
	for _, file := range value.Files {
		media := file.Lesson().Document.Pages[0].Nodes[0].Nodes[1]
		if media.Type != lesson.NodeMedia || len(media.Data) != 4 {
			t.Fatalf("wrong media: %+v", media)
		}
		for index, name := range []string{"Clip.MP3", "Clip.txt", "Clip.vtt", "Clip.srt"} {
			embedded, ok := media.Data[index].Value.(lesson.EmbeddedFile)
			if !ok || embedded.FileName != name {
				t.Fatalf("wrong media order: %+v", media.Data)
			}
			decoded, err := base64.StdEncoding.DecodeString(embedded.Content)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, files["nested/"+name]) {
				t.Fatalf("embedded bytes changed: %s", name)
			}
		}
	}
	for path, raw := range sourceRaw {
		integrationSameBytes(t, path, raw)
	}
	for name, raw := range files {
		integrationSameBytes(t, filepath.Join(base, "media", filepath.FromSlash(name)), raw)
	}
}

func TestIntegrationGoldenResourceScopesDoNotSearchAdjacentFiles(t *testing.T) {
	for _, number := range []int{1, 3} {
		t.Run(fmt.Sprint(number), func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "source")
			path := filepath.Join(source, "lesson.json")
			original := integrationRead(t, filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", number)))
			integrationWrite(t, path, original)
			integrationWrite(t, filepath.Join(source, "12.01.mp3"), []byte{0, 1, 2})
			integrationWrite(t, filepath.Join(source, "003.mp4"), []byte{3, 4, 5})
			op, issues := app.NewCreate(base, source, "Course", "")
			integrationOK(t, issues)
			issues = op.Analyze(integrationAt())
			integrationCode(t, issues, resourcefind.CodeNotFound)
			if op.Snapshot().Actions.ReviewSave || !op.Snapshot().Actions.Retry {
				t.Fatal("missing resource not blocking/retryable")
			}
			integrationSameBytes(t, path, original)
		})
	}
}

func TestIntegrationRetryGetsFreshResourceTreeAndClearsDecisions(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	media := filepath.Join(base, "media", "nested")
	unused := filepath.Join(media, "unused.bin")
	integrationWrite(t, unused, []byte{9})
	sourceRaw := map[string][]byte{}
	for _, id := range []string{"RETRY1", "RETRY2"} {
		path := filepath.Join(source, id+".json")
		integrationWriteInput(t, path, integrationMediaInput(id))
		sourceRaw[path] = integrationRead(t, path)
	}
	op, issues := app.NewCreate(base, source, "Initial course", "")
	integrationOK(t, issues)
	integrationCode(t, op.Analyze(integrationAt()), resourcefind.CodeNotFound)
	failed := op.Snapshot()
	if !failed.Actions.Retry || failed.Actions.Save || failed.Actions.ReviewSave || failed.Module != nil || failed.Catalog != nil {
		t.Fatalf("source error did not block the whole operation: %+v", failed)
	}
	clip := filepath.Join(media, "Clip.mp3")
	clipBytes := []byte{1, 2, 3}
	integrationWrite(t, clip, clipBytes)
	integrationOK(t, op.Retry(integrationAt()))
	integrationCode(t, op.SetDestination(failed.Token, base, "stale"), app.CodeStaleReview)
	ready := op.Snapshot()
	if !ready.ManifestReady || len(ready.Blockers) != 0 || ready.Actions.Retry || len(ready.Module.Files) != 2 || ready.Resources.Total != 2 || ready.Resources.Used != 1 || ready.Resources.Unused != 1 {
		t.Fatalf("first Retry did not rebuild the shared resource union: %+v", ready)
	}
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Edited")
	integrationOK(t, issues)
	integrationOK(t, op.AcceptEdit(edit.Token))
	integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "old-target"))
	review, issues := op.ReviewSave(op.Snapshot().Token)
	integrationOK(t, issues)
	integrationOK(t, op.ConfirmSave(review.Token, true, false))
	healthy := op.Snapshot()
	integrationCode(t, op.Retry(integrationAt()), app.CodeState)
	unchanged := op.Snapshot()
	if healthy.Actions.Retry || !healthy.Actions.Save || unchanged.Token != healthy.Token || unchanged.Actions != healthy.Actions || unchanged.Target == nil || *unchanged.Target != *healthy.Target || unchanged.Catalog.CourseTitle != "Edited" || unchanged.Resources.Total != 2 || unchanged.Resources.Used != 1 || unchanged.WriteResult != nil {
		t.Fatal("rejected healthy Retry changed the confirmed plan")
	}
	// Принятый конфликт весов делает Retry применимым по §16.5.
	edit, issues = op.ProposeLessonWeight(unchanged.Token, "l002.json", "01")
	integrationCode(t, issues, catalog.CodeWeightConflict)
	if edit == nil || len(edit.Blockers) == 0 {
		t.Fatal("missing editable weight conflict")
	}
	integrationCode(t, op.AcceptEdit(edit.Token), catalog.CodeWeightConflict)
	blocked := op.Snapshot()
	integrationCode(t, blocked.Blockers, catalog.CodeWeightConflict)
	if !blocked.Actions.Retry || blocked.Actions.Save || blocked.Actions.ReviewSave || blocked.ManifestReady || len(blocked.Conflicts) != 1 || blocked.Target == nil {
		t.Fatalf("Retry precondition is not a current catalog blocker: %+v", blocked)
	}
	pending, issues := op.ProposeCourseTitle(blocked.Token, "Pending title")
	integrationCode(t, issues, catalog.CodeWeightConflict)
	if pending == nil {
		t.Fatal("missing pending edit before Retry")
	}
	// Тот же inode по новому пути отличает новый Resolver от прежнего кеша.
	moved := filepath.Join(media, "MovedClip.mp3")
	if err := os.Rename(clip, moved); err != nil {
		t.Fatal(err)
	}
	newUnused := filepath.Join(media, "new-unused.bin")
	integrationWrite(t, newUnused, []byte{8})
	integrationOK(t, op.Retry(integrationAt()))
	snapshot := op.Snapshot()
	if snapshot.Target != nil || snapshot.Catalog.CourseTitle != "Initial course" || snapshot.Actions.Save || snapshot.Actions.Retry || snapshot.WriteResult != nil || !snapshot.ManifestReady || len(snapshot.Blockers) != 0 || snapshot.Resources.Total != 3 || snapshot.Resources.Used != 1 || snapshot.Resources.Unused != 2 || len(snapshot.Resources.UsedPaths) != 1 || snapshot.Resources.UsedPaths[0] != moved {
		t.Fatalf("Retry kept old analysis: %+v", snapshot)
	}
	for index, item := range snapshot.Catalog.Items {
		if item.Entry.Order == nil || *item.Entry.Order != fmt.Sprint(index+1) {
			t.Fatal("Retry kept accepted weight decisions")
		}
	}
	integrationCode(t, op.AcceptEdit(pending.Token), app.CodeStaleReview)
	integrationCode(t, op.ConfirmSave(review.Token, true, false), app.CodeStaleReview)
	integrationCode(t, op.SetDestination(blocked.Token, base, "stale"), app.CodeStaleReview)
	_, issues = op.Save(integrationAt())
	if len(issues) == 0 {
		t.Fatal("Retry kept write consent")
	}
	integrationAbsent(t, filepath.Join(base, "old-target.zip"))
	integrationOK(t, op.SetDestination(snapshot.Token, base, "fresh"))
	value := integrationSave(t, op, false)
	if len(value.Files) != 2 || value.Manifest.Course.Title != "Initial course" {
		t.Fatal("fresh ZIP retained the old plan")
	}
	for _, file := range value.Files {
		embedded := file.Lesson().Document.Pages[0].Nodes[0].Nodes[1].Data[0].Value.(lesson.EmbeddedFile)
		decoded, err := base64.StdEncoding.DecodeString(embedded.Content)
		if err != nil || embedded.FileName != "MovedClip.mp3" || !bytes.Equal(decoded, clipBytes) {
			t.Fatal("fresh ZIP did not use the renamed resource bytes")
		}
	}
	for path, raw := range sourceRaw {
		integrationSameBytes(t, path, raw)
	}
	integrationSameBytes(t, moved, clipBytes)
	integrationSameBytes(t, unused, []byte{9})
	integrationSameBytes(t, newUnused, []byte{8})
	integrationAbsent(t, clip)
}
