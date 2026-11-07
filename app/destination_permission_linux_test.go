package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestDestinationInspectionDistinguishesPermissionFromAbsence(t *testing.T) {
	for _, exists := range []bool{false, true} {
		name := "absent"
		if exists {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "target.zip")
			if exists {
				destinationWrite(t, path, []byte("protected"))
			}
			target := destinationObserve(t, path)
			op := &Operation{mode: Create}
			destinationConfirmed(t, op, target)
			if err := os.Chmod(dir, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(dir, 0700); err != nil {
					t.Error(err)
				}
			})
			if _, err := os.Lstat(path); !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("real permission-denied setup did not deny Lstat: %v", err)
			}
			got, problem := inspectDestination(path)
			if got != nil || problem == nil || problem.Code != CodeDestination || !errors.Is(problem, fs.ErrPermission) {
				t.Fatalf("permission denial mistaken for absent destination: %#v, %#v", got, problem)
			}
			problem = op.checkDestination()
			if problem == nil || problem.Code != CodeTargetChanged || op.saving != nil {
				t.Fatalf("unverifiable target kept consent: %#v", problem)
			}
		})
	}
}
