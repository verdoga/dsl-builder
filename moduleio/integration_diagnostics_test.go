package moduleio_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

func TestIntegrationDelegatesAllMetadataAndPlacement(t *testing.T) {
	for _, mode := range []string{"six-metadata", "missing-order", "mixed-placement", "duplicate-weight"} {
		t.Run(mode, func(t *testing.T) {
			model := IntegrationModel(t, IntegrationNew(t, "l001.json", "AB88"), IntegrationNew(t, "l002.json", "CD99"))
			entries := model.Manifest.Course.Lessons
			switch mode {
			case "six-metadata":
				entry := &(*entries)[0]
				subtitle := ""
				entry.ID, entry.Title, entry.Subtitle, entry.LineCount, entry.CreatedAt, entry.Version = "Other", "Changed", &subtitle, "1", "2026-10-01T00:00:00Z", "2"
			case "missing-order":
				*entries = (*entries)[:1]
				model.Files = model.Files[:1]
				(*entries)[0].Order = nil
			case "mixed-placement":
				*model.Manifest.Course.Sections = []manifest.Section{{Title: "Section", Lessons: []manifest.Entry{(*entries)[1]}}}
				*entries = (*entries)[:1]
			case "duplicate-weight":
				duplicate := "01"
				(*entries)[1].Order = &duplicate
			}
			expected := module.Validate(model, diagnostic.Context{Path: "candidate.zip"})
			if len(expected) == 0 {
				t.Fatal("owner accepted invalid fixture")
			}
			if mode == "six-metadata" {
				count := 0
				for _, problem := range expected {
					if problem.Code == module.CodeMetadata {
						count++
					}
				}
				if count != 6 {
					t.Fatalf("metadata fixture: %v", expected)
				}
			}
			var dst bytes.Buffer
			written := moduleio.Write(&dst, model, diagnostic.Context{Path: "candidate.zip"})
			if dst.Len() != 0 || len(written) != len(expected) {
				t.Fatalf("validation diagnostics lost or partial write: %v", written)
			}
			for i, problem := range expected {
				if written[i].Code != problem.Code || written[i].Reason != problem.Reason {
					t.Fatalf("owner code/reason changed: %v / %v", expected, written)
				}
			}
			zipEntries := []IntegrationEntry{{Name: "manifest.json", Raw: IntegrationJSON(t, model.Manifest)}}
			for _, file := range model.Files {
				zipEntries = append(zipEntries, IntegrationEntry{Name: file.FileName(), Raw: IntegrationJSON(t, file.Lesson())})
			}
			path := IntegrationPath(t, IntegrationZIP(t, zipEntries...))
			value, readProblems := moduleio.Read(path)
			if value != nil || len(readProblems) != len(expected) {
				t.Fatalf("partial model or diagnostics: %v %v", value, readProblems)
			}
			for i, want := range expected {
				got := readProblems[i]
				if got.Code != want.Code || got.Reason != want.Reason || got.Context.Path != path {
					t.Fatalf("lost owner/context: %+v", got)
				}
				if len(got.Details) != len(want.Details) || len(got.Related) != len(want.Related) {
					t.Fatalf("lost owner details/related: %+v / %+v", got, want)
				}
				for j, detail := range want.Details {
					if got.Details[j] != detail {
						t.Fatalf("changed owner details: %+v", got)
					}
				}
			}
			if problems := moduleio.Verify(path); len(problems) != len(expected) {
				t.Fatalf("Verify did not repeat full Read: %v", problems)
			}
		})
	}
}

func TestIntegrationIndependentEntryDiagnosticsAndNameCause(t *testing.T) {
	model := IntegrationModel(t, IntegrationNew(t, "l001.json", "AB88"))
	archive := IntegrationZIP(t,
		IntegrationEntry{Name: "l002.json", Raw: []byte(`{"document":{}}`)},
		IntegrationEntry{Name: "manifest.json", Raw: []byte(`{"extra":"secret-body-marker"}`)},
		IntegrationEntry{Name: "l001.json", Raw: []byte(`{"formatVersion":"1.0"}`)},
	)
	path := IntegrationPath(t, archive)
	value, problems := moduleio.Read(path)
	if value != nil || len(problems) < 3 {
		t.Fatalf("independent errors lost: %v %v", value, problems)
	}
	previous := -1
	seen := [3]bool{}
	names := []string{"l002.json", "manifest.json", "l001.json"}
	for _, problem := range problems {
		index := -1
		for i, name := range names {
			if problem.Context.FileName == name {
				index = i
			}
		}
		if index < previous || index < 0 || problem.Context.Path != path {
			t.Fatalf("entry error order/context: %+v", problem)
		}
		previous = index
		seen[index] = true
		if bytes.Contains([]byte(problem.Error()), []byte("secret-body-marker")) {
			t.Fatal("JSON body leaked")
		}
	}
	if !seen[0] || !seen[1] || !seen[2] {
		t.Fatal("entry omitted", seen)
	}
	bad := IntegrationZIP(t, IntegrationEntry{Name: "manifest.json", Raw: IntegrationJSON(t, model.Manifest)}, IntegrationEntry{Name: "l000.json", Raw: []byte(`{}`)})
	problem := IntegrationReject(t, bad, moduleio.CodeEntryName)
	var owner *diagnostic.Error
	if !errors.As(problem.Cause, &owner) || owner.Code != identity.CodeInvalidFileName {
		t.Fatalf("identity cause lost: %+v", problem)
	}
}
