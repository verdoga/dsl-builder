package resourceusage_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/resourceusage"
)

func registryAssertState(t *testing.T, registry *resourceusage.Registry, used, unused []string) {
	t.Helper()
	report := registry.Snapshot()
	if report.Total != len(used)+len(unused) || report.Used != len(used) || report.Unused != len(unused) {
		t.Errorf("counters = (%d, %d, %d), want (%d, %d, %d)", report.Total, report.Used, report.Unused, len(used)+len(unused), len(used), len(unused))
	}
	for _, paths := range []struct {
		name string
		got  []string
		want []string
	}{
		{"used", report.UsedPaths, used},
		{"unused", report.UnusedPaths, unused},
	} {
		got, want := slices.Clone(paths.got), slices.Clone(paths.want)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s paths = %q, want %q", paths.name, got, want)
		}
	}
}

func TestRegistryZeroValueAndEmptyOperations(t *testing.T) {
	var registry resourceusage.Registry
	registryAssertState(t, &registry, nil, nil)
	registry.Register(nil)
	registry.Register([]string{})
	registry.MarkUsed(nil)
	registry.MarkUsed([]string{})
	registry.Merge(nil)
	registry.Merge(&registry)
	var empty resourceusage.Registry
	registry.Merge(&empty)
	registryAssertState(t, &registry, nil, nil)
	registry.Register([]string{"/resources/new.mp3"})
	registryAssertState(t, &registry, nil, []string{"/resources/new.mp3"})
}

func TestRegistryRegisterKeepsExactUniquePaths(t *testing.T) {
	longPath := `\\?\C:\Учебные материалы\` + strings.Repeat(`длинный каталог\`, 25) + `Трек 01.mp3`
	paths := []string{
		`C:\Материалы\SB\Track 01.mp3`,
		`C:\Материалы\WB\Track 01.mp3`,
		`C:\Материалы\SB\Track 01.txt`,
		`C:\Материалы\SB\unused.unknown`,
		`C:\Материалы\SB\README`,
		longPath,
		"/resources/Audio/Track.mp3",
		"/resources/Audio/track.mp3",
	}
	var registry resourceusage.Registry
	registry.Register(paths)
	registry.Register([]string{paths[0], paths[0], paths[1], longPath})
	registryAssertState(t, &registry, nil, paths)
}

func TestRegistryMarkUsedIsMonotonicAndAddsUnknownPaths(t *testing.T) {
	const registered = "/resources/registered.mp3"
	const unknown = "/external/selected.mp4"
	const unused = "/resources/not-selected.unknown"
	var registry resourceusage.Registry
	registry.Register([]string{registered, unused, registered})
	registryAssertState(t, &registry, nil, []string{registered, unused})
	registry.MarkUsed([]string{registered, unknown, unknown})
	registryAssertState(t, &registry, []string{registered, unknown}, []string{unused})
	registry.Register([]string{unknown, registered, unused})
	registry.MarkUsed([]string{unknown, registered, registered})
	registry.Register(nil)
	registry.MarkUsed([]string{})
	registryAssertState(t, &registry, []string{registered, unknown}, []string{unused})
}

func TestRegistryMarkUsedOnZeroValue(t *testing.T) {
	var registry resourceusage.Registry
	registry.MarkUsed([]string{"/external/clip.mp4", "/external/clip.mp4"})
	registryAssertState(t, &registry, []string{"/external/clip.mp4"}, nil)
}

func TestRegistryDoesNotRetainInputSlices(t *testing.T) {
	for _, operation := range []string{"Register", "MarkUsed"} {
		t.Run(operation, func(t *testing.T) {
			paths := []string{"/resources/z.mp3", "/resources/a.mp3", "/resources/z.mp3"}
			original := slices.Clone(paths)
			var registry resourceusage.Registry
			if operation == "Register" {
				registry.Register(paths)
			} else {
				registry.MarkUsed(paths)
			}
			for i := range paths {
				paths[i] = "/resources/replaced.mp3"
			}
			want := []string{original[0], original[1]}
			if operation == "Register" {
				registryAssertState(t, &registry, nil, want)
			} else {
				registryAssertState(t, &registry, want, nil)
			}
		})
	}
}

func TestRegistryMergeUsesLogicalOR(t *testing.T) {
	for _, test := range []struct {
		name      string
		leftUsed  bool
		rightUsed bool
	}{
		{"unused_and_unused", false, false},
		{"unused_and_used", false, true},
		{"used_and_unused", true, false},
		{"used_and_used", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const shared = "/resources/shared.mp3"
			var left, right resourceusage.Registry
			left.Register([]string{shared, "/left/unused.txt"})
			left.MarkUsed([]string{"/left/used.mp3"})
			right.Register([]string{shared, "/right/unused.txt"})
			right.MarkUsed([]string{"/right/used.mp3"})
			if test.leftUsed {
				left.MarkUsed([]string{shared})
			}
			if test.rightUsed {
				right.MarkUsed([]string{shared})
			}
			left.Merge(&right)
			left.Merge(&right)
			used := []string{"/left/used.mp3", "/right/used.mp3"}
			unused := []string{"/left/unused.txt", "/right/unused.txt"}
			if test.leftUsed || test.rightUsed {
				used = append(used, shared)
			} else {
				unused = append(unused, shared)
			}
			registryAssertState(t, &left, used, unused)
			rightUsed := []string{"/right/used.mp3"}
			rightUnused := []string{"/right/unused.txt"}
			if test.rightUsed {
				rightUsed = append(rightUsed, shared)
			} else {
				rightUnused = append(rightUnused, shared)
			}
			registryAssertState(t, &right, rightUsed, rightUnused)
		})
	}
}

func TestRegistryMergeKeepsRegistriesIndependent(t *testing.T) {
	for _, initiallyEmpty := range []bool{true, false} {
		name := "populated_receiver"
		if initiallyEmpty {
			name = "zero_receiver"
		}
		t.Run(name, func(t *testing.T) {
			var target, source resourceusage.Registry
			var targetUnused []string
			if !initiallyEmpty {
				targetUnused = append(targetUnused, "/target/original.txt")
				target.Register(targetUnused)
			}
			source.Register([]string{"/source/unused.txt"})
			source.MarkUsed([]string{"/source/used.mp3"})
			target.Merge(&source)
			target.MarkUsed([]string{"/source/unused.txt", "/target/new.mp3"})
			target.Register([]string{"/target/new.txt"})
			registryAssertState(t, &source, []string{"/source/used.mp3"}, []string{"/source/unused.txt"})
			source.MarkUsed([]string{"/source/new.mp3"})
			source.Register([]string{"/source/new.txt"})
			registryAssertState(t, &target,
				[]string{"/source/used.mp3", "/source/unused.txt", "/target/new.mp3"},
				append(targetUnused, "/target/new.txt"))
		})
	}
}

func TestRegistryMergeNilEmptyAndSelfPreserveState(t *testing.T) {
	var registry, empty resourceusage.Registry
	registry.Register([]string{"/resources/unused.txt"})
	registry.MarkUsed([]string{"/resources/used.mp3"})
	registry.Merge(nil)
	registry.Merge(&empty)
	registry.Merge(&registry)
	registry.Register(nil)
	registry.Register([]string{})
	registry.MarkUsed(nil)
	registry.MarkUsed([]string{})
	registryAssertState(t, &registry, []string{"/resources/used.mp3"}, []string{"/resources/unused.txt"})
	registryAssertState(t, &empty, nil, nil)
}
