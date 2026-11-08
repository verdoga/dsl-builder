package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
)

func integrationCurrentGolden(nodes []lesson.Node) {
	for index := range nodes {
		if nodes[index].Type == lesson.NodeMultifill {
			nodes[index].Flags.Checkable = nil
		}
		integrationCurrentGolden(nodes[index].Nodes)
	}
}

func TestIntegrationCreateUnmodifiedNonMediaGoldenToActualZIP(t *testing.T) {
	for _, number := range []int{2, 4, 5, 6, 7} {
		t.Run(fmt.Sprint(number), func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "source")
			path := filepath.Join(source, "lesson.JSON")
			raw := integrationRead(t, filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", number)))
			integrationWrite(t, path, raw)
			op := integrationCreate(t, base, source)
			integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "golden"))
			value := integrationSave(t, op, false)
			actual := integrationFile(t, value, "l001.json").Lesson()
			expectedPath := filepath.Join("..", "golden", fmt.Sprintf("%d. lesson.json", number))
			if number == 6 {
				// Независимый эталон учитывает инструкцию перед matching и порядок выдачи ID.
				expectedPath = filepath.Join("..", "convert", "testdata", "integration", "golden-6-tz.expected.json")
			}
			var expected lesson.Lesson
			if err := json.Unmarshal(integrationRead(t, expectedPath), &expected); err != nil {
				t.Fatal(err)
			}
			expected.Document.CreatedAt = integrationAt().UTC().Format("2006-01-02T15:04:05Z")
			for index := range expected.Document.Pages {
				integrationCurrentGolden(expected.Document.Pages[index].Nodes)
			}
			want, err := json.Marshal(expected)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("golden %d model differs after TZ10.7 multifill/date normalization", number)
			}
			integrationSameBytes(t, path, raw)
		})
	}
}
