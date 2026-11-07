package moduleio_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
)

func TestGoldenNormativeFixtureApplicable(t *testing.T) {
	value := GoldenLesson(t, GoldenRead(t, "testdata/golden/storage.lesson.json"))
	GoldenDataTypes(t, value)
	document := value.Document
	if document.ID != "GoLdEn" || document.Version != "9007199254740993e0" || document.LineCount != "9007199254740995.0" || document.CreatedAt != "2020-02-29T12:34:56Z" {
		t.Fatal("prepared IDs, number lexemes or date changed")
	}
	if document.Title != " Кириллица <b> & текст " || document.Subtitle == nil || *document.Subtitle != "" {
		t.Fatal("exact title or empty subtitle changed")
	}
	if len(document.Pages) != 2 || document.Pages[0].ID != "golden-p1001" || document.Pages[1].ID != "golden-p002" || document.Pages[1].Nodes == nil || len(document.Pages[1].Nodes) != 0 {
		t.Fatal("prepared page order or empty array changed")
	}
	nodes := document.Pages[0].Nodes[0].Nodes[0].Nodes
	media := nodes[0].Data
	if len(media) != 3 {
		t.Fatal("body/script/subtitles must remain separate data")
	}
	for index, name := range []string{"pixel.png", "script.txt", "captions.vtt"} {
		file := media[index].Value.(lesson.EmbeddedFile)
		raw, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			t.Fatal(err)
		}
		if file.FileName != name || !bytes.Equal(raw, GoldenRead(t, "testdata/golden/resources/"+name)) {
			t.Fatalf("embedded %s differs from the explicit source bytes", name)
		}
	}
	if nodes[1].Type != lesson.NodeScript || nodes[1].Data[0].Value != "Исходная расшифровка\nБез рендеринга" {
		t.Fatal("source script must remain an independent node")
	}
	if nodes[2].Data[0].Value != "I _____{am} Camila.\n\nWe _____{are} from Spain." || nodes[2].Flags.Checkable != nil {
		t.Fatal("opaque multifill or absent checkable changed")
	}
	if nodes[3].Data[0].Value != "header | header\n---\nа | b\nc | d" {
		t.Fatal("table text was rendered or changed")
	}
	children := nodes[4].Data[0].Value.([]lesson.Data)
	if children[0].Value != "second" || len(children[0].Answers) != 2 || children[0].Answers[0] != "b" || children[0].Answers[1] != "a" {
		t.Fatal("nested collection or answer array order changed")
	}
}

func GoldenEqualLesson(t *testing.T, got, want *lesson.Lesson) {
	t.Helper()
	GoldenDataTypes(t, got)
	actual, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("known lesson model, number lexemes or array order changed")
	}
}
