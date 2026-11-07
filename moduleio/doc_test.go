package moduleio_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestDocWriteLeavesTransactionAndStreamWithCaller(t *testing.T) {
	model, original := DocModel(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "цель.zip")
	before := []byte("существующая цель")
	if err := os.WriteFile(target, before, 0600); err != nil {
		t.Fatal(err)
	}
	stream := &DocStream{}
	context := diagnostic.Context{Path: target}
	var callback safewrite.WriteFunc = func(dst io.Writer) error {
		return DocJoin(moduleio.Write(dst, model, context))
	}
	if stream.Len() != 0 || stream.writes != 0 {
		t.Fatal("I/O before callback")
	}
	if err := callback(stream); err != nil {
		t.Fatal(err)
	}
	if stream.closes != 0 || stream.syncs != 0 {
		t.Fatalf("caller stream closed/synced: %d/%d", stream.closes, stream.syncs)
	}
	archive, err := zip.NewReader(bytes.NewReader(stream.Bytes()), int64(stream.Len()))
	if err != nil {
		t.Fatalf("Write did not finalize ZIP: %v", err)
	}
	var lessonRaw []byte
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("incomplete ZIP entry: %v / %v", readErr, closeErr)
		}
		if file.Name == "l001.json" {
			lessonRaw = raw
		}
	}
	if !bytes.Equal(lessonRaw, original) {
		t.Fatal("loaded lesson was reserialized")
	}
	after, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("Write changed context.Path: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
		t.Fatalf("Write changed transaction directory: %v / %v", entries, err)
	}
	if _, err := stream.Write([]byte("caller still owns stream")); err != nil {
		t.Fatal(err)
	}
	if err := DocJoin(nil); err != nil {
		t.Fatalf("successful empty diagnostics became error: %v", err)
	}
}

func TestDocWriteCallbackPreservesDiagnosticsAndCauses(t *testing.T) {
	model, _ := DocModel(t)
	context := diagnostic.Context{Path: "кандидат.zip", Line: 17, Column: 4}
	t.Run("invalid model preserves owner and source", func(t *testing.T) {
		second, problems := module.NewFile("l001.json", model.Files[0].Lesson(), diagnostic.Context{
			Path: "второй источник.json", Line: 31, Column: 2,
		})
		if len(problems) != 0 {
			t.Fatal(problems)
		}
		candidate := *model
		candidate.Files = []*module.File{model.Files[0], second}
		stream := &DocStream{}
		var received []*diagnostic.Error
		var callback safewrite.WriteFunc = func(dst io.Writer) error {
			received = moduleio.Write(dst, &candidate, context)
			return DocJoin(received)
		}
		joined := callback(stream)
		DocJoined(t, joined, received)
		if stream.writes != 0 || stream.closes != 0 || stream.syncs != 0 {
			t.Fatal("invalid model touched caller stream")
		}
		for _, problem := range received {
			if problem.Code != module.CodeDuplicateFile {
				continue
			}
			if len(problem.Details) == 0 || !slices.Contains(problem.Related, model.Files[0].Context()) ||
				!slices.Contains(problem.Related, second.Context()) {
				t.Fatalf("source context lost: %#v", problem)
			}
			return
		}
		t.Fatal("owner duplicate-file diagnostic missing")
	})
	t.Run("writer cause stays reachable", func(t *testing.T) {
		cause := errors.New("поток отказал")
		stream := &DocStream{failure: cause}
		problems := moduleio.Write(stream, model, context)
		joined := DocJoin(problems)
		DocJoined(t, joined, problems)
		if !errors.Is(joined, cause) {
			t.Fatal("writer cause lost")
		}
		for _, problem := range problems {
			if problem.Context.Path != context.Path {
				t.Fatalf("operation path lost: %#v", problem.Context)
			}
		}
		if stream.closes != 0 || stream.syncs != 0 {
			t.Fatal("failed Write closed or synced caller stream")
		}
	})
}

func TestDocReadAndVerifyAreReadOnlyAndCloseDescriptors(t *testing.T) {
	for _, operation := range []string{"Read", "Verify"} {
		for _, scenario := range []string{"success", "entry failure", "CRC failure"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				invalid := scenario != "success"
				dir := t.TempDir()
				path := filepath.Join(dir, "кандидат.zip")
				var before []byte
				if scenario == "CRC failure" {
					before = DocCorruptArchive(t, path)
				} else {
					before = DocArchive(t, path, invalid)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				var problems []*diagnostic.Error
				if operation == "Read" {
					var model *module.Module
					model, problems = moduleio.Read(path)
					if (model == nil) != invalid {
						t.Fatalf("Read returned partial/missing model: %v", problems)
					}
				} else {
					var callback safewrite.VerifyFunc = func(path string) error {
						problems = moduleio.Verify(path)
						return DocJoin(problems)
					}
					joined := callback(path)
					if invalid {
						DocJoined(t, joined, problems)
					} else if joined != nil {
						t.Fatal(joined)
					}
				}
				if scenario == "CRC failure" {
					DocJoined(t, DocJoin(problems), problems)
					if !errors.Is(DocJoin(problems), zip.ErrChecksum) {
						t.Fatal("CRC failure cause lost")
					}
				} else if invalid {
					DocCheckArchiveDiagnostics(t, path, problems)
				} else if len(problems) != 0 {
					t.Fatal(problems)
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("archive changed: %v", err)
				}
				afterInfo, err := os.Stat(path)
				if err != nil || !os.SameFile(info, afterInfo) || !info.ModTime().Equal(afterInfo.ModTime()) || info.Mode() != afterInfo.Mode() {
					t.Fatalf("archive replaced or metadata changed: %v", err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 1 {
					t.Fatalf("archive extracted/installed files: %v / %v", entries, err)
				}
				DocClosed(t, path)
			})
		}
	}
}

func DocCheckArchiveDiagnostics(t *testing.T, path string, problems []*diagnostic.Error) {
	t.Helper()
	DocJoined(t, DocJoin(problems), problems)
	var duplicate, identityCause bool
	for _, problem := range problems {
		if problem.Context.Path != path || problem.Context.Line != 0 || problem.Context.Column != 0 {
			t.Fatalf("unknown source context invented/lost: %#v", problem.Context)
		}
		if problem.Code == moduleio.CodeDuplicateEntry {
			duplicate = len(problem.Related) >= 2 && len(problem.Details) != 0
		}
		if problem.Code == moduleio.CodeEntryName && problem.Context.FileName == "L001.json" {
			var cause *diagnostic.Error
			identityCause = errors.As(problem.Cause, &cause) && cause.Code == identity.CodeInvalidFileName
		}
	}
	if !duplicate || !identityCause {
		t.Fatalf("independent structured diagnostics missing: %v", problems)
	}
}
