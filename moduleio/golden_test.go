package moduleio_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
)

func TestGoldenLessonStorageRoundTrips(t *testing.T) {
	paths := []string{"testdata/golden/storage.lesson.json"}
	for i := 1; i <= 7; i++ {
		paths = append(paths, fmt.Sprintf("../golden/%d. lesson.json", i))
	}
	for _, source := range paths {
		t.Run(source, func(t *testing.T) {
			raw := GoldenRead(t, source)
			prepared := GoldenPrepared(t, raw)
			want := prepared.Files[0].Lesson()
			newRaw, err := json.MarshalIndent(want, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			newPath := GoldenWritten(t, prepared)
			GoldenAssertOutput(t, newPath, prepared, [][]byte{newRaw})
			GoldenEqualLesson(t, GoldenLoaded(t, newPath).Files[0].Lesson(), want)

			// Raw-эталон — исходные байты, а не сериализация загруженной модели.
			input := GoldenZIP(t, []GoldenEntry{
				{"l001.json", raw}, {module.ManifestFileName, GoldenJSON(t, prepared.Manifest)},
			})
			inputBytes := GoldenRead(t, input)
			loaded := GoldenLoaded(t, input)
			if !bytes.Equal(loaded.Files[0].OriginalBytes(), raw) {
				t.Fatal("Read did not retain exact source lesson bytes")
			}
			GoldenEqualLesson(t, loaded.Files[0].Lesson(), want)
			// Изменение выданных копий не должно нарушать связь снимка с raw.
			exposedRaw := loaded.Files[0].OriginalBytes()
			exposedRaw[0] ^= 1
			exposedModel := loaded.Files[0].Lesson()
			exposedModel.Document.Title = "Изменена только внешняя копия"
			GoldenEqualLesson(t, loaded.Files[0].Lesson(), want)
			output := GoldenWritten(t, loaded)
			GoldenAssertOutput(t, output, loaded, [][]byte{raw})
			GoldenEqualLesson(t, GoldenLoaded(t, output).Files[0].Lesson(), want)
			if !bytes.Equal(GoldenRead(t, input), inputBytes) {
				t.Fatal("reading or verifying changed the source ZIP")
			}

			loaded.Manifest.CreatedAt = "2026-11-06T23:45:01Z"
			loaded.Manifest.Course.Title = "Изменён только манифест"
			manifestOnly := GoldenWritten(t, loaded)
			GoldenAssertOutput(t, manifestOnly, loaded, [][]byte{raw})
			GoldenEqualLesson(t, GoldenLoaded(t, manifestOnly).Files[0].Lesson(), want)
		})
	}
}

func TestGoldenPreparedReplacementAndAddition(t *testing.T) {
	raw := GoldenRead(t, "../golden/1. lesson.json")
	prepared := GoldenPrepared(t, raw)
	path := GoldenZIP(t, []GoldenEntry{
		{module.ManifestFileName, GoldenJSON(t, prepared.Manifest)}, {"l001.json", raw},
	})
	loaded := GoldenLoaded(t, path)
	changed := loaded.Files[0].Lesson()
	changed.Document.Title = "Подготовленная новая версия"
	changed.Document.Version = "2"
	changed.Document.CreatedAt = "2026-11-06T01:02:03Z"
	replaced, problems := module.ReplaceFile(loaded.Files[0], changed, diagnostic.Context{Path: "changed.json"})
	GoldenNoProblems(t, problems)
	added := GoldenLesson(t, GoldenRead(t, "../golden/2. lesson.json"))
	newFile, problems := module.NewFile("l1001.json", added, diagnostic.Context{Path: "added.json"})
	GoldenNoProblems(t, problems)
	if replaced.OriginalBytes() != nil || newFile.OriginalBytes() != nil {
		t.Fatal("prepared replacement and addition must require serialization")
	}
	entries := *loaded.Manifest.Course.Lessons
	order := *entries[0].Order
	entries[0] = GoldenEntryFor(t, replaced.FileName(), changed.Document, order)
	entries = append(entries, GoldenEntryFor(t, newFile.FileName(), added.Document, order+"1"))
	loaded.Manifest.Course.Lessons = &entries
	loaded.Manifest.CreatedAt = "2026-11-06T01:03:04Z"
	loaded.Files = []*module.File{newFile, replaced}
	GoldenNoProblems(t, module.Validate(loaded, diagnostic.Context{}))
	addedRaw, err := json.MarshalIndent(added, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	changedRaw, err := json.MarshalIndent(changed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	output := GoldenWritten(t, loaded)
	GoldenAssertOutput(t, output, loaded, [][]byte{addedRaw, changedRaw})
	reread := GoldenLoaded(t, output)
	GoldenEqualLesson(t, reread.Files[0].Lesson(), added)
	GoldenEqualLesson(t, reread.Files[1].Lesson(), changed)
	if !bytes.Equal(GoldenLoaded(t, path).Files[0].OriginalBytes(), raw) {
		t.Fatal("prepared changes altered the original archive")
	}
}
