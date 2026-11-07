package app

import (
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
)

func TestDestinationInputPathUsesExplicitBaseAndOneQuotePair(t *testing.T) {
	base := t.TempDir()
	absolute := filepath.Join(base, "Абсолютный путь", "file.json")
	cases := []struct{ input, unwrapped string }{
		{"relative folder/file.json", "relative folder/file.json"},
		{`"relative folder/file.json"`, "relative folder/file.json"},
		{absolute, absolute}, {`"` + absolute + `"`, absolute},
		{`""folder""`, `"folder"`},
		{`"unpaired`, `"unpaired`}, {`unpaired"`, `unpaired"`},
		{" folder ", " folder "},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			want, ownerProblem := filetree.Resolve(base, tc.unwrapped)
			got, problem := inputPath(base, tc.input)
			if ownerProblem != nil {
				if problem == nil || problem.Code != ownerProblem.Code {
					t.Fatalf("owner error %v was not preserved: %q, %v", ownerProblem, got, problem)
				}
				return
			}
			if problem != nil || got != want {
				t.Fatalf("inputPath(%q) = %q, %v; want %q", tc.input, got, problem, want)
			}
		})
	}
}

func TestDestinationInputPathKeepsResourceOwnerSemanticsSeparate(t *testing.T) {
	base := t.TempDir()
	interactive, problem := inputPath(base, `"media clip.wav"`)
	if problem != nil || interactive != filepath.Join(base, "media clip.wav") {
		t.Fatalf("interactive outer quotes were not removed: %q, %v", interactive, problem)
	}
	resource, ownerProblem := filetree.Resolve(base, `"media clip.wav"`)
	if ownerProblem == nil && resource == interactive {
		t.Fatal("filetree resource path unexpectedly used interactive unquoting")
	}
	for _, value := range []string{"", `""`, "a\x00b"} {
		_, got := inputPath(base, value)
		if got == nil || got.Code != filetree.CodePath {
			t.Fatalf("inputPath(%q) = %#v; want owner path error", value, got)
		}
	}
	_, problem = inputPath("relative-base", "file.json")
	if problem == nil || problem.Code != filetree.CodePath {
		t.Fatalf("relative base accepted: %#v", problem)
	}
}
