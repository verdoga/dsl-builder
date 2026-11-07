package convert

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestResourcesFillMediaEmbedsExactBytesRolesAndOrder(t *testing.T) {
	root := t.TempDir()
	files := []struct {
		name string
		mime string
		role lesson.DataRole
	}{
		{"Клип 003.MP4", "video/mp4", lesson.RoleBody},
		{"Клип 003.txt", "text/plain", lesson.RoleScript},
		{"Клип 003.vtt", "text/vtt", lesson.RoleSubtitles},
		{"Клип 003.srt", "application/x-subrip", lesson.RoleSubtitles},
		{"Клип 003.ttml", "application/ttml+xml", lesson.RoleSubtitles},
		{"Клип 003.dfxp", "application/ttml+xml", lesson.RoleSubtitles},
		{"Клип 003.ass", "text/plain", lesson.RoleSubtitles},
		{"Клип 003.ssa", "text/plain", lesson.RoleSubtitles},
	}
	opaque := []byte{0xef, 0xbb, 0xbf, 0, 0xff, '\r', '\n', 1, 2, 3}
	for _, file := range files {
		resourcesWrite(t, filepath.Join(root, "media", file.name), opaque)
	}
	resourcesWrite(t, filepath.Join(root, "media", "unused.bin"), []byte("not selected"))
	input, tree := resourcesLoad(t, root, []string{"media"},
		resourcesMediaLine(t, 1, "video", "003", "003"),
		resourcesLine(t, 2, model.LineTypeTag, resourcesElement(t, model.ElementTypeTag, "@script", "script"),
			resourcesElement(t, model.ElementTypeContent, "исходная расшифровка", "исходная расшифровка")))
	pages, bindings := resourcesBindings(t, input)
	if len(bindings) != 2 || bindings[1].Node(pages).Type != lesson.NodeScript {
		t.Fatal("expected separate source script binding")
	}
	beforeInput := resourcesJSON(t, input)
	beforeBinding := resourcesJSON(t, bindings)
	beforeScript := resourcesJSON(t, bindings[1].Node(pages))
	node := bindings[0].Node(pages)
	beforeFlags := resourcesJSON(t, node.Flags)
	beforeChildren := resourcesJSON(t, node.Nodes)
	usage := &resourceusage.Registry{}
	finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
	if problem != nil {
		t.Fatal(problem)
	}
	if report := usage.Snapshot(); report.Total != 9 || report.Used != 0 {
		t.Fatalf("inventory already used: %+v", report)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if problem := fillMedia(node, bindings[0], finder, usage, resourcesContext(input)); problem != nil {
			t.Fatal(problem)
		}
		if len(node.Data) != len(files) {
			t.Fatalf("got %d data, want %d independent roles", len(node.Data), len(files))
		}
		var used []string
		for i, file := range files {
			datum := node.Data[i]
			value, ok := datum.Value.(lesson.EmbeddedFile)
			if !ok || datum.Type != lesson.DataMedia || datum.Role == nil || *datum.Role != file.role || datum.Answers == nil || len(datum.Answers) != 0 || datum.Hint != "" {
				t.Fatalf("data[%d] has wrong shape: %+v", i, datum)
			}
			decoded, err := base64.StdEncoding.DecodeString(value.Content)
			if err != nil || !bytes.Equal(decoded, opaque) || value.Content != base64.StdEncoding.EncodeToString(opaque) || value.FileName != file.name || value.MIMEType != file.mime {
				t.Fatalf("data[%d] changed selected bytes/name/MIME: %+v", i, value)
			}
			used = append(used, resourcesIdentity(t, tree, filepath.Join(root, "media", file.name)))
		}
		slices.Sort(used)
		report := usage.Snapshot()
		if report.Total != 9 || report.Used != 8 || report.Unused != 1 || !slices.Equal(report.UsedPaths, used) {
			t.Fatalf("incorrect successful usage: %+v", report)
		}
	}
	if resourcesJSON(t, input) != beforeInput || resourcesJSON(t, bindings) != beforeBinding || resourcesJSON(t, bindings[1].Node(pages)) != beforeScript || resourcesJSON(t, node.Flags) != beforeFlags || resourcesJSON(t, node.Nodes) != beforeChildren {
		t.Fatal("fillMedia changed source, binding, script neighbour, flags or children")
	}
}

func TestResourcesFillMediaDelegatesExactRawAndValue(t *testing.T) {
	for _, scenario := range []struct {
		name, raw, value, body string
		dirs                   []string
	}{
		{"quoted-raw-value-path", `"ignored raw path"`, "outside/Фото.png", "outside/Фото.png", nil},
		{"leading-zero-search", "003", "003", "media/name 003 clip.png", []string{"media"}},
		{"decimal-search", "12.01", "12.01", "media/track12.01.png", []string{"media"}},
		{"spaces-preserved", " x y ", " x y ", "media/a x y z.png", []string{"media"}},
		{"empty-search", "", "", "media/only.png", []string{"media"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			resourcesWrite(t, filepath.Join(root, scenario.body), []byte{})
			resourcesWrite(t, filepath.Join(root, "outside", "unused.bin"), []byte("unused"))
			resourcesWrite(t, filepath.Join(root, scenario.body+".txt"), []byte("image companion must not embed"))
			input, tree := resourcesLoad(t, root, scenario.dirs, resourcesMediaLine(t, 1, "image", scenario.raw, scenario.value))
			pages, bindings := resourcesBindings(t, input)
			usage := &resourceusage.Registry{}
			finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
			if problem != nil {
				t.Fatal(problem)
			}
			node := bindings[0].Node(pages)
			if problem := fillMedia(node, bindings[0], finder, usage, resourcesContext(input)); problem != nil {
				t.Fatal(problem)
			}
			if len(node.Data) != 1 {
				t.Fatalf("image got companions: %+v", node.Data)
			}
			value, ok := node.Data[0].Value.(lesson.EmbeddedFile)
			if !ok || value.FileName != filepath.Base(scenario.body) || value.MIMEType != "image/png" || value.Content != "" {
				t.Fatalf("unexpected image: %+v", node.Data[0])
			}
			report := usage.Snapshot()
			if report.Used != 1 || !slices.Equal(report.UsedPaths, []string{resourcesIdentity(t, tree, filepath.Join(root, scenario.body))}) {
				t.Fatalf("unexpected selected paths: %+v", report)
			}
			if scenario.dirs == nil && report.Total != 1 {
				t.Fatalf("explicit selection accounted unrelated neighbours: %+v", report)
			}
		})
	}
}
