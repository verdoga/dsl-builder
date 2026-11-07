package resourcefind_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/resourcefind"
)

func TestResourcefindIntegrationGoldenSourcesWithRealTemporaryResources(t *testing.T) {
	for _, tc := range []struct {
		number, line            int
		kind, value, name, mime string
	}{
		{1, 35, "audio", "12.01", "12.01.mp3", "audio/mpeg"},
		{3, 36, "video", "003", "003.mp4", "video/mp4"},
	} {
		t.Run(fmt.Sprintf("golden%d", tc.number), func(t *testing.T) {
			path := filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", tc.number))
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var input struct {
				Document struct {
					FilePath string
					Metadata struct {
						DocumentID   string
						ResourceDirs []string
					}
				}
				Lines []struct {
					Line     int
					Elements []struct {
						Type, Raw, Value string
						Start            int
					}
				}
			}
			if err := json.Unmarshal(bytes, &input); err != nil {
				t.Fatal(err)
			}
			if len(input.Document.Metadata.ResourceDirs) != 0 {
				t.Fatalf("historical resourceDirs changed: %q", input.Document.Metadata.ResourceDirs)
			}
			var raw, value, kind string
			column := 0
			for _, line := range input.Lines {
				if line.Line != tc.line {
					continue
				}
				for _, element := range line.Elements {
					if element.Type == "source" {
						raw, value, column = element.Raw, element.Value, element.Start
					}
					if element.Type == "media-type" {
						kind = element.Value
					}
				}
			}
			if raw != tc.value || value != tc.value || kind != tc.kind || column != 14 {
				t.Fatalf("golden source = kind=%q raw=%q value=%q column=%d", kind, raw, value, column)
			}
			root := t.TempDir()
			jsonPath := rfiWrite(t, root, "technical JSON/A.json")
			body := rfiWrite(t, root, "technical JSON/resources/"+tc.name)
			var wanted []string
			wanted = append(wanted, body)
			if tc.number == 3 {
				wanted = append(wanted, rfiWrite(t, root, "technical JSON/resources/003.txt"), rfiWrite(t, root, "technical JSON/resources/003.vtt"))
			}
			ctx := rfiContext(jsonPath)
			ctx.LessonID, ctx.Line, ctx.Column = input.Document.Metadata.DocumentID, tc.line, column
			tree := new(filetree.Resolver)
			historical, accounted := rfiPrepare(t, tree, jsonPath, input.Document.Metadata.ResourceDirs)
			if len(accounted) != 0 {
				t.Errorf("historical scope unexpectedly populated: %q", accounted)
			}
			_, findErr := historical.Find(kind, raw, value, ctx)
			rfiError(t, findErr, resourcefind.CodeNotFound, ctx, kind, value)
			// Область задаётся отдельно в тестовом вызове; golden остаётся исходным.
			f, inventory := rfiPrepare(t, tree, jsonPath, []string{"resources"})
			rfiPaths(t, inventory, wanted)
			got := rfiFind(t, f, kind, raw, value, ctx)
			if got.Body.Entry.Path != body || got.Body.MIMEType != tc.mime {
				t.Fatalf("golden body = %+v", got.Body)
			}
			rfiPaths(t, got.AccountedPaths, wanted)
			if tc.number == 1 && (got.Script != nil || len(got.Subtitles) != 0) {
				t.Errorf("invented companions: %+v", got)
			}
			if tc.number == 3 && (got.Script == nil || len(got.Subtitles) != 1) {
				t.Errorf("golden3 companions missing: %+v", got)
			}
			if input.Document.FilePath == jsonPath {
				t.Fatal("fixture no longer distinguishes original DSL from technical JSON")
			}
		})
	}
}
