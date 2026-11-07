package filetree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func resolverFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("not JSON\x00\xff"), 0600); err != nil {
		t.Fatal(err)
	}
}

func resolverStat(t *testing.T, path string) fs.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func resolverInspect(t *testing.T, r *Resolver, path string) Entry {
	t.Helper()
	entry, err := r.Inspect(path)
	if err != nil {
		t.Fatalf("Inspect(%q): %+v", path, err)
	}
	if entry.Path != filepath.Clean(path) || entry.Info == nil || !os.SameFile(entry.Info, resolverStat(t, path)) {
		t.Fatalf("incorrect discovery path or target metadata: %+v", entry)
	}
	return entry
}

func resolverError(t *testing.T, err *diagnostic.Error, code, path string, cause error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s for %q", code, path)
	}
	if err.Code != code || err.Context.Path != path || !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Fatalf("incorrect diagnostic: %+v", err)
	}
	if cause != nil && (err.Cause == nil || !errors.Is(err, cause)) {
		t.Fatalf("lost original cause %v: %+v", cause, err)
	}
}

func TestResolverInspectPreservesDiscoveryAndResolvesTargets(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "Ресурсы с пробелом", strings.Repeat("сегмент-", 10), strings.Repeat("segment-", 14))
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "Цель.bin")
	if len(target) <= 260 {
		t.Fatal("fixture must exercise a path longer than 260 bytes")
	}
	resolverFile(t, target)
	fileLink, dirLink := filepath.Join(root, "Обращение.JSON"), filepath.Join(root, "База JSON")
	for link, destination := range map[string]string{fileLink: target, dirLink: targetDir} {
		if err := os.Symlink(destination, link); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	var r Resolver
	for _, path := range []string{fileLink, target, filepath.Join(dirLink, "Цель.bin")} {
		entry := resolverInspect(t, &r, path)
		if entry.IdentityPath != resolved || !entry.Info.Mode().IsRegular() {
			t.Fatalf("target identity/type: %+v; want %q", entry, resolved)
		}
	}
	entry := resolverInspect(t, &r, dirLink)
	if !entry.Info.IsDir() || entry.IdentityPath != filepath.Dir(resolved) {
		t.Fatalf("directory link must describe target: %+v", entry)
	}
	if runtime.GOOS == "windows" {
		t.Run("WindowsCaseAndSeparators", func(t *testing.T) {
			for _, path := range []string{strings.ToUpper(target), filepath.ToSlash(target)} {
				if got := resolverInspect(t, &r, path); got.IdentityPath != resolved {
					t.Fatalf("same Windows object changed identity: %+v", got)
				}
			}
		})
	}
}

func TestResolverHardLinksKeepFirstPathWithinOneResolver(t *testing.T) {
	root := t.TempDir()
	first, alias, other := filepath.Join(root, "Z.bin"), filepath.Join(root, "A.bin"), filepath.Join(root, "other.bin")
	resolverFile(t, first)
	resolverFile(t, other)
	if err := os.Link(first, alias); err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(resolverStat(t, first), resolverStat(t, alias)) || os.SameFile(resolverStat(t, first), resolverStat(t, other)) {
		t.Fatal("fixture does not have the required OS identities")
	}
	var r Resolver
	for _, path := range []string{first, alias, first} {
		if got := resolverInspect(t, &r, path); got.IdentityPath != first {
			t.Fatalf("first identity path changed: %+v", got)
		}
	}
	if got := resolverInspect(t, &r, other); got.IdentityPath != other {
		t.Fatalf("distinct object merged: %+v", got)
	}
	var fresh Resolver
	if got := resolverInspect(t, &fresh, alias); got.IdentityPath != alias {
		t.Fatalf("independent resolver inherited another operation: %+v", got)
	}
}

func TestResolverFreshMetadataAndDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "изменяемый.bin")
	resolverFile(t, path)
	var r Resolver
	before := resolverInspect(t, &r, path)
	if err := os.WriteFile(path, []byte("changed to a different size"), 0600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2021, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	after := resolverInspect(t, &r, path)
	current := resolverStat(t, path)
	if after.IdentityPath != before.IdentityPath || after.Info.Size() != current.Size() || after.Info.Size() == before.Info.Size() || !after.Info.ModTime().Equal(current.ModTime()) {
		t.Fatalf("cached metadata returned: before=%+v after=%+v", before.Info, after.Info)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, err := r.Inspect(path)
	resolverError(t, err, CodeAccess, path, fs.ErrNotExist)
}

func TestResolverDistinctCaseOnCaseSensitiveFilesystem(t *testing.T) {
	root := t.TempDir()
	upper, lower := filepath.Join(root, "Case.bin"), filepath.Join(root, "case.bin")
	resolverFile(t, upper)
	file, err := os.OpenFile(lower, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, fs.ErrExist) {
		t.Log("filesystem is case insensitive; distinct case objects cannot be created here")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if os.SameFile(resolverStat(t, upper), resolverStat(t, lower)) {
		t.Fatal("fixture must contain distinct objects")
	}
	var r Resolver
	first, second := resolverInspect(t, &r, upper), resolverInspect(t, &r, lower)
	if first.IdentityPath == second.IdentityPath {
		t.Fatalf("case distinct objects merged: %+v %+v", first, second)
	}
}

func TestResolverRetargetedLinkUsesFreshObject(t *testing.T) {
	root := t.TempDir()
	first, second, link := filepath.Join(root, "first.bin"), filepath.Join(root, "second.bin"), filepath.Join(root, "link.bin")
	resolverFile(t, first)
	resolverFile(t, second)
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	var r Resolver
	before := resolverInspect(t, &r, link)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	after := resolverInspect(t, &r, link)
	if before.IdentityPath != first || after.IdentityPath != second || os.SameFile(before.Info, after.Info) {
		t.Fatalf("retargeted link reused previous object: before=%+v after=%+v", before, after)
	}
}

func TestResolverInvalidPathsAndLinkFailures(t *testing.T) {
	var r Resolver
	for _, path := range []string{"", "relative.json"} {
		_, err := r.Inspect(path)
		resolverError(t, err, CodePath, path, nil)
		_, _, err = resolveExisting(path)
		resolverError(t, err, CodePath, path, nil)
	}
	root := t.TempDir()
	missing, loop, broken := filepath.Join(root, "missing"), filepath.Join(root, "loop"), filepath.Join(root, "broken")
	if err := os.Symlink("loop", loop); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", broken); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, code string }{{missing, CodeAccess}, {loop, CodeLink}, {broken, CodeLink}} {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			_, err := r.Inspect(tc.path)
			resolverError(t, err, tc.code, tc.path, nil)
			if err.Cause == nil {
				t.Errorf("Inspect(%q) lost its filesystem cause", tc.path)
			}
			_, _, err = resolveExisting(tc.path)
			resolverError(t, err, tc.code, tc.path, nil)
			if err.Cause == nil {
				t.Errorf("resolveExisting(%q) lost its filesystem cause", tc.path)
			}
		})
	}
}

func TestResolveExistingReturnsCleanTargetAndMetadata(t *testing.T) {
	root := t.TempDir()
	target, link := filepath.Join(root, "target"), filepath.Join(root, "alias")
	resolverFile(t, target)
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, link, root} {
		want, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		got, info, diagnostic := resolveExisting(path)
		if diagnostic != nil || got != want || !filepath.IsAbs(got) || info == nil || !os.SameFile(info, resolverStat(t, path)) {
			t.Fatalf("resolveExisting(%q) = %q, %+v, %v; want %q", path, got, info, diagnostic, want)
		}
	}
}

func TestResolverIdentifyUsesOSFileIdentity(t *testing.T) {
	root := t.TempDir()
	first, alias, other := filepath.Join(root, "first"), filepath.Join(root, "alias"), filepath.Join(root, "other")
	resolverFile(t, first)
	resolverFile(t, other)
	if err := os.Link(first, alias); err != nil {
		t.Fatal(err)
	}
	var r Resolver
	for _, path := range []string{first, alias, other, alias} {
		want := first
		if !os.SameFile(resolverStat(t, first), resolverStat(t, path)) {
			want = other
		}
		if got := r.identify(path, resolverStat(t, path)); got != want {
			t.Fatalf("identify(%q) = %q; want %q", path, got, want)
		}
	}
}

func TestResolverMetadataWithoutReadingAndFreshAccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Log("Unix mode bits cannot establish Windows ACL denial; ACL runtime coverage is separate")
		return
	}
	root := t.TempDir()
	dir := filepath.Join(root, "closed")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unreadable")
	resolverFile(t, path)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if file, err := os.Open(path); !errors.Is(err, fs.ErrPermission) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("fixture must deny byte reads, got %v", err)
	}
	var r Resolver
	resolverInspect(t, &r, path)
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	_, err := r.Inspect(path)
	resolverError(t, err, CodeAccess, path, fs.ErrPermission)
}
