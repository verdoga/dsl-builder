package convert

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestMetadataNewLessonPreservesKnownValues(t *testing.T) {
	for _, title := range []string{"", "Название", "  Заголовок\tΩ\n"} {
		for _, subtitle := range []*string{nil, metadataString(""), metadataString("  Подзаголовок Ω\t")} {
			name := strconv.Quote(title) + "/null"
			if subtitle != nil {
				name = strconv.Quote(title) + "/" + strconv.Quote(*subtitle)
			}
			t.Run(name, func(t *testing.T) {
				document := metadataDocument(title, subtitle, 42)
				before := metadataJSON(t, document)
				pointers := document
				context := diagnostic.Context{Path: "/source/technical.json", FileName: "l007.json", LessonID: "EfX7"}
				got, err := newLesson(document, context)
				metadataUnchanged(t, document, pointers, before)
				if err != nil || got == nil {
					t.Fatalf("newLesson = %v, %v; want shell without error", got, err)
				}
				want := lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
					ID: "EfX7", Title: title, Subtitle: subtitle,
					LineCount: json.Number("42"), Pages: []lesson.Page{},
				}}
				if !bytes.Equal(metadataJSON(t, got), metadataJSON(t, want)) {
					t.Fatalf("shell = %s; want %s", metadataJSON(t, got), metadataJSON(t, want))
				}
				if got.Document.Pages == nil || len(got.Document.Pages) != 0 {
					t.Fatalf("Pages = %#v; want nonnil empty slice", got.Document.Pages)
				}
				if got.Document.Version != "" || got.Document.CreatedAt != "" {
					t.Fatalf("revision assigned early: version=%q, createdAt=%q", got.Document.Version, got.Document.CreatedAt)
				}
				if subtitle != nil && got.Document.Subtitle == subtitle {
					t.Fatal("Subtitle aliases input pointer")
				}
			})
		}
	}
}

func TestMetadataNewLessonLineCountIsExactDecimal(t *testing.T) {
	maxInt := "9223372036854775807"
	if strconv.IntSize == 32 {
		maxInt = "2147483647"
	}
	values := []string{"0", "1", "73", maxInt}
	if strconv.IntSize == 64 {
		values = append(values, "9007199254740993", "9007199254740995")
	}
	for _, decimal := range values {
		t.Run(decimal, func(t *testing.T) {
			count, err := strconv.Atoi(decimal)
			if err != nil {
				t.Fatal(err)
			}
			document := metadataDocument("", nil, count)
			before := metadataJSON(t, document)
			got, problem := newLesson(document, diagnostic.Context{})
			metadataUnchanged(t, document, document, before)
			if problem != nil || got == nil {
				t.Fatalf("newLesson = %v, %v; known %s is allowed", got, problem, decimal)
			}
			if got.Document.LineCount != json.Number(decimal) {
				t.Fatalf("LineCount = %q; want exact decimal %s", got.Document.LineCount, decimal)
			}
			if got.Document.Title != "" {
				t.Fatalf("known empty Title replaced with %q", got.Document.Title)
			}
			if got.Document.Pages == nil || len(got.Document.Pages) != 0 {
				t.Fatal("shell must not derive pages from lineCount")
			}
			var wire struct {
				Document struct {
					LineCount json.RawMessage `json:"lineCount"`
				} `json:"document"`
			}
			if err := json.Unmarshal(metadataJSON(t, got), &wire); err != nil {
				t.Fatal(err)
			}
			if string(wire.Document.LineCount) != decimal {
				t.Fatalf("serialized lineCount = %s; want %s", wire.Document.LineCount, decimal)
			}
		})
	}
}

func TestMetadataNewLessonRejectsUnknownRequiredValues(t *testing.T) {
	for _, missing := range []string{"title", "lineCount", "both"} {
		t.Run(missing, func(t *testing.T) {
			document := metadataDocument("Known", metadataString("Subtitle"), 0)
			if missing == "title" || missing == "both" {
				document.Metadata.Title = nil
			}
			if missing == "lineCount" || missing == "both" {
				document.LineCount = nil
			}
			before := metadataJSON(t, document)
			pointers := document
			context := diagnostic.Context{Path: "/JSON/вход.json", FileName: "l023.json", LessonID: "EfX7"}
			got, problem := newLesson(document, context)
			metadataUnchanged(t, document, pointers, before)
			if got != nil || problem == nil {
				t.Fatalf("newLesson = %v, %v; unknown %s must reject whole shell", got, problem, missing)
			}
			if problem.Code != "convert.metadata" || problem.Reason == "" {
				t.Fatalf("diagnostic = %#v; want CodeMetadata and reason", problem)
			}
			if problem.Context != context {
				t.Fatalf("diagnostic context = %#v; want %#v", problem.Context, context)
			}
			description := strings.ToLower(problem.Reason)
			for _, detail := range problem.Details {
				description += " " + strings.ToLower(detail.Label+" "+detail.Value)
			}
			field := strings.ToLower(missing)
			if missing == "both" {
				if !strings.Contains(description, "title") && !strings.Contains(description, "linecount") {
					t.Fatalf("missing field not identified: %#v", problem)
				}
			} else if !strings.Contains(description, field) {
				t.Fatalf("missing %s not identified: %#v", missing, problem)
			}
		})
	}
}

func TestMetadataNewLessonResultsOwnTheirPointers(t *testing.T) {
	document := metadataDocument("Title", metadataString("Subtitle"), 73)
	before := metadataJSON(t, document)
	first, firstErr := newLesson(document, diagnostic.Context{})
	second, secondErr := newLesson(document, diagnostic.Context{})
	if firstErr != nil || secondErr != nil || first == nil || second == nil {
		t.Fatalf("newLesson errors: %v, %v", firstErr, secondErr)
	}
	if first == second || first.Document.Subtitle == nil || second.Document.Subtitle == nil {
		t.Fatal("results must be independent and retain known Subtitle")
	}
	if first.Document.Subtitle == second.Document.Subtitle || first.Document.Subtitle == document.Metadata.Subtitle || second.Document.Subtitle == document.Metadata.Subtitle {
		t.Fatal("results share mutable Subtitle pointers")
	}
	*first.Document.Subtitle = "Result edit"
	first.Document.Title = "Other title"
	first.Document.Pages = append(first.Document.Pages, lesson.Page{ID: "caller-only"})
	metadataUnchanged(t, document, document, before)
	if second.Document.Title != "Title" || *second.Document.Subtitle != "Subtitle" || len(second.Document.Pages) != 0 {
		t.Fatalf("editing first result changed second: %#v", second.Document)
	}
	*document.Metadata.Subtitle = "Source edit"
	*document.Metadata.Title = "Source title"
	*document.Metadata.DocumentID = "Else"
	*document.LineCount = 5
	if *second.Document.Subtitle != "Subtitle" || second.Document.Title != "Title" || second.Document.ID != "EfX7" || second.Document.LineCount != "73" {
		t.Fatalf("editing source changed second: %#v", second.Document)
	}
}

func metadataString(value string) *string { return &value }

func metadataDocument(title string, subtitle *string, count int) model.Document {
	byteLength, bom := int64(12345), true
	return model.Document{
		DSLVersion: metadataString("1.2"), FileName: metadataString("Do not derive title.txt"),
		FilePath: metadataString(`C:\Other\old-source.txt`), Encoding: metadataString("UTF-8"),
		HasBOM: &bom, LineCount: &count, ByteLength: &byteLength, SHA256: metadataString(strings.Repeat("a", 64)),
		Metadata: model.DocumentMetadata{
			DocumentID: metadataString("EfX7"), Title: metadataString(title), Subtitle: subtitle,
			Section: metadataString("Case Sensitive"), Order: metadataString("0000123"),
			ResourceDirs: []string{"resources", "Other directory"},
		},
	}
}

func metadataJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func metadataUnchanged(t *testing.T, got, pointers model.Document, before []byte) {
	t.Helper()
	if !bytes.Equal(metadataJSON(t, got), before) {
		t.Fatalf("input changed: %s; before %s", metadataJSON(t, got), before)
	}
	if got.DSLVersion != pointers.DSLVersion || got.FileName != pointers.FileName || got.FilePath != pointers.FilePath ||
		got.Encoding != pointers.Encoding || got.HasBOM != pointers.HasBOM || got.LineCount != pointers.LineCount ||
		got.ByteLength != pointers.ByteLength || got.SHA256 != pointers.SHA256 ||
		got.Metadata.DocumentID != pointers.Metadata.DocumentID || got.Metadata.Title != pointers.Metadata.Title ||
		got.Metadata.Subtitle != pointers.Metadata.Subtitle || got.Metadata.Section != pointers.Metadata.Section || got.Metadata.Order != pointers.Metadata.Order {
		t.Fatal("input pointer identity changed")
	}
}
