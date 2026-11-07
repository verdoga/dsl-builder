package app

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func docRetryMedia(title, name string) model.Result {
	result := docFixtureInput("DR01", title, "", "", "10", "body")
	result.Document.Metadata.ResourceDirs = []string{"media"}
	count := len(result.Lines) + 1
	result.Lines = append(result.Lines, model.Line{
		Number: count, LineType: "tag", Raw: "@media audio " + name, LineEnding: "\n",
		Elements: []model.Element{
			{ElementType: "tag", Raw: "@media", Value: docFixtureText("media"), Start: 1, End: 7, ErrorIDs: []string{}},
			{ElementType: "media-type", Raw: "audio", Value: docFixtureText("audio"), Start: 8, End: 13, ErrorIDs: []string{}},
			{ElementType: "source", Raw: name, Value: docFixtureText(name), Start: 14, End: 14 + len(name), ErrorIDs: []string{}},
		},
	})
	result.Document.LineCount = &count
	return result
}

func TestDocRetryRereadsJSONResourcesAndClearsOldTargetAndTokens(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "in", "synthetic-media.json")
	mediaDirectory := filepath.Join(filepath.Dir(source), "media")
	if err := os.MkdirAll(mediaDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	unused := filepath.Join(mediaDirectory, "unused.bin")
	if err := os.WriteFile(unused, []byte{4, 5}, 0600); err != nil {
		t.Fatal(err)
	}
	docFixtureWrite(t, source, docRetryMedia("First", "needle"))
	secondSource := filepath.Join(filepath.Dir(source), "second.json")
	docFixtureWrite(t, secondSource, docFixtureInput("DR02", "Other", "", "", "10", "other body"))
	op, issues := NewCreate(base, filepath.Dir(source), "Course", "")
	docFixtureOK(t, issues)
	docFixtureCode(t, op.Analyze(docFixtureAt()), "resourcefind.not_found")
	failed := op.Snapshot()
	if !failed.Actions.Retry || failed.Actions.Save || failed.Actions.ReviewSave {
		t.Fatal("source failure lost safe continuation actions")
	}
	oldMedia := filepath.Join(mediaDirectory, "needle.mp3")
	if err := os.WriteFile(oldMedia, []byte{0, 1, 255}, 0600); err != nil {
		t.Fatal(err)
	}
	docFixtureCode(t, op.Retry(docFixtureAt().Add(time.Hour)), catalog.CodeWeightConflict)
	first := op.Snapshot()
	if first.Resources.Total != 2 || first.Resources.Used != 1 || first.Resources.Unused != 1 || first.Catalog == nil || !first.Actions.Retry || first.Actions.ReviewSave {
		t.Fatal("retry did not load corrected real resource")
	}
	docFixtureOK(t, op.SetDestination(first.Token, base, "abandoned"))
	beforeRetry := op.Snapshot()
	if err := os.Remove(oldMedia); err != nil {
		t.Fatal(err)
	}
	newMedia := filepath.Join(mediaDirectory, "second.mp3")
	mediaBytes := []byte{9, 8, 7, 0, 255}
	if err := os.WriteFile(newMedia, mediaBytes, 0600); err != nil {
		t.Fatal(err)
	}
	inputBytes := docFixtureWrite(t, source, docRetryMedia("Second", "second"))
	docFixtureWrite(t, secondSource, docFixtureInput("DR02", "Other", "", "", "20", "other body"))
	docFixtureOK(t, op.Retry(docFixtureAt().Add(2*time.Hour)))
	second := op.Snapshot()
	if second.Target != nil || second.Actions.Save || second.Module.Files[1].Lesson().Document.Title != "Second" || second.Resources.Total != 2 || second.Resources.Used != 1 || second.Resources.UsedPaths[0] != newMedia {
		t.Fatal("retry mixed previous source/registry/target state with fresh analysis")
	}
	docFixtureCode(t, op.SetDestination(beforeRetry.Token, base, "stale"), CodeStaleReview)
	if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
		t.Fatal("retry retained old save consent")
	}
	docFixtureAbsent(t, filepath.Join(base, "abandoned.zip"))
	docFixtureOK(t, op.SetDestination(second.Token, base, "fresh"))
	loaded := docFixtureSave(t, op, false)
	if loaded.Files[1].Lesson().Document.Title != "Second" || !bytes.Equal(inputBytes, docFixtureBytes(t, source)) || !bytes.Equal(mediaBytes, docFixtureBytes(t, newMedia)) {
		t.Fatal("fresh lifecycle changed inputs or saved stale model")
	}
	nodes := loaded.Files[1].Lesson().Document.Pages[0].Nodes
	embedded, ok := nodes[len(nodes)-1].Data[0].Value.(lesson.EmbeddedFile)
	if !ok {
		t.Fatal("saved media lost its embedded value")
	}
	decoded, err := base64.StdEncoding.DecodeString(embedded.Content)
	if err != nil || !bytes.Equal(decoded, mediaBytes) {
		t.Fatal("real ZIP round trip changed resource bytes")
	}
}

func TestDocOriginalGoldenMediaHasNoImplicitNeighborSearch(t *testing.T) {
	for _, example := range []struct{ number, media string }{{"1", "12.01.mp3"}, {"3", "003.mp4"}} {
		t.Run(example.number, func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "lesson.json")
			original := docFixtureBytes(t, filepath.Join("..", "golden", example.number+". parsed.json"))
			if err := os.WriteFile(source, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(base, example.media), []byte{1, 2, 3}, 0600); err != nil {
				t.Fatal(err)
			}
			op, issues := NewCreate(base, base, "Course", "")
			docFixtureOK(t, issues)
			docFixtureCode(t, op.Analyze(docFixtureAt()), "resourcefind.not_found")
			if snapshot := op.Snapshot(); snapshot.Actions.ReviewSave || !snapshot.Actions.Retry {
				t.Fatal("failed original golden unexpectedly became writable")
			}
			if !bytes.Equal(original, docFixtureBytes(t, source)) {
				t.Fatal("analysis repaired original golden input on disk")
			}
		})
	}
}
