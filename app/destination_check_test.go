package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func destinationObserve(t *testing.T, path string) *targetState {
	t.Helper()
	value, problem := inspectDestination(path)
	if problem != nil || value == nil {
		t.Fatalf("cannot observe %q: %v", path, problem)
	}
	return value
}

func destinationConfirmed(t *testing.T, op *Operation, target *targetState) *saveReview {
	t.Helper()
	op.key = &reviewKey{owner: op}
	op.target = target
	shown := *target
	op.saving = &saveReview{
		key: &reviewKey{owner: op}, base: op.key, target: &shown,
		confirmed: true, replaceExisting: true,
	}
	return op.saving
}

func TestDestinationCheckUnchangedKeepsConsentAndObservation(t *testing.T) {
	for _, mode := range []Mode{Create, Update, Add} {
		existence := []bool{true}
		if mode == Create {
			existence = append(existence, false)
		}
		for _, exists := range existence {
			t.Run(string(mode)+"/"+map[bool]string{false: "absent", true: "existing"}[exists], func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "target.zip")
				if exists {
					destinationWrite(t, path, []byte("unchanged bytes"))
				}
				target := destinationObserve(t, path)
				op := &Operation{mode: mode, archivePath: path}
				consent := destinationConfirmed(t, op, target)
				for attempt := 0; attempt < 2; attempt++ {
					if problem := op.checkDestination(); problem != nil {
						t.Fatalf("unchanged destination rejected: %v", problem)
					}
					if op.saving != consent || !consent.confirmed || !consent.replaceExisting || op.target != target {
						t.Fatal("unchanged check revoked consent or replaced its observation")
					}
				}
				if exists {
					destinationAssertBytes(t, path, []byte("unchanged bytes"))
				} else if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("check wrote absent target: %v", err)
				}
			})
		}
	}
}

func TestDestinationCheckRejectsChangesAndRevokesAllConsent(t *testing.T) {
	for _, mode := range []Mode{Create, Update, Add} {
		for _, change := range []string{"appearance", "disappearance", "replacement", "size", "mtime", "directory", "symlink"} {
			t.Run(string(mode)+"/"+change, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "target.zip")
				if change != "appearance" {
					destinationWrite(t, path, []byte("old"))
				}
				target := destinationObserve(t, path)
				op := &Operation{mode: mode, archivePath: path, analyzed: true}
				destinationConfirmed(t, op, target)
				switch change {
				case "appearance":
					destinationWrite(t, path, []byte("new"))
				case "disappearance":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				case "replacement":
					replacement := filepath.Join(dir, "replacement.zip")
					destinationWrite(t, replacement, []byte("new"))
					at := target.info.ModTime()
					if err := os.Chtimes(replacement, at, at); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(path, filepath.Join(dir, "original.zip")); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, path); err != nil {
						t.Fatal(err)
					}
					if os.SameFile(target.info, destinationInfo(t, path)) {
						t.Fatal("replacement setup retained old identity")
					}
				case "size":
					destinationWrite(t, path, []byte("larger content"))
					at := target.info.ModTime()
					if err := os.Chtimes(path, at, at); err != nil {
						t.Fatal(err)
					}
				case "mtime":
					at := target.info.ModTime().Add(10 * time.Second)
					if err := os.Chtimes(path, at, at); err != nil {
						t.Fatal(err)
					}
				case "directory", "symlink":
					if err := os.Rename(path, filepath.Join(dir, "original.zip")); err != nil {
						t.Fatal(err)
					}
					var err error
					if change == "directory" {
						err = os.Mkdir(path, 0700)
					} else {
						err = os.Symlink(filepath.Join(dir, "original.zip"), path)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if change == "replacement" || change == "size" || change == "mtime" {
					observed := destinationInfo(t, path)
					same := os.SameFile(target.info, observed)
					if change == "replacement" && (same || target.info.Size() != observed.Size() || !target.info.ModTime().Equal(observed.ModTime())) {
						t.Fatal("replacement setup did not isolate file identity")
					}
					if change == "size" && (!same || target.info.Size() == observed.Size() || !target.info.ModTime().Equal(observed.ModTime())) {
						t.Fatal("size setup did not isolate size change")
					}
					if change == "mtime" && (!same || target.info.Size() != observed.Size() || target.info.ModTime().Equal(observed.ModTime())) {
						t.Fatal("mtime setup did not isolate modification time")
					}
				}
				problem := op.checkDestination()
				if problem == nil || problem.Code != CodeTargetChanged || problem.Reason == "" {
					t.Fatalf("changed destination = %#v; want CodeTargetChanged", problem)
				}
				if op.saving != nil {
					t.Fatal("changed destination retained final consent/overwrite authorization")
				}
				if mode != Create {
					if op.target == nil || op.target.info != target.info {
						t.Fatal("Update/Add replaced original baseline observation")
					}
					if problem := op.checkDestination(); problem == nil || problem.Code != CodeTargetChanged {
						t.Fatalf("recheck legitimized changed Update/Add baseline: %#v", problem)
					}
				}
			})
		}
	}
}
