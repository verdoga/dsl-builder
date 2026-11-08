package structure_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/structure"
	"github.com/verdoga/dsl-builder/textdata"
)

func TestTask26GoldenStructuralProjectionsAndConsumerCompatibility(t *testing.T) {
	// Ожидания заданы по ТЗ 10/19: исходный script — своя нода;
	// данные, ID и checkable исторических lesson здесь не сравниваются.
	cases := []struct {
		number     int
		shape      string
		firstLines []int
		titles     []*string
	}{
		{1, "[task(instruction,multifill,answerKey),task(media,instruction)]", []int{7, 8, 8, 21, 34, 35, 36}, []*string{task26String("1 GRAMMAR: Be going to: positive and negative")}},
		{2, "[task(instruction,example,alt,multifill),note,alt]", []int{6, 7, 11, 12, 13, 16, 20}, []*string{nil}},
		{3, "[heading,task(instruction,example,singleChoice,singleChoice,question,question)]|[heading,task(step(media,script,instruction),step(wordList,instruction,multifill))]", []int{5, 6, 8, 13, 18, 23, 28, 29, 32, 34, 35, 36, 37, 41, 42, 43, 44, 44}, []*string{nil, nil}},
		{4, "[wordList]|[table]|[text]", []int{6, 14, 23}, []*string{task26String("Title A"), task26String("Title B"), task26String("Title C")}},
		{5, "[task(question)]|[task(instruction,multifill)]|[task(instruction,singleChoice)]", []int{8, 9, 14, 15, 16, 23, 24, 25}, []*string{task26String("Title A"), task26String("Title B"), task26String("Title C")}},
		{6, "[task(instruction,multipleChoice,hint),task(instruction,multipleChoice),task(instruction,matching,hint),task(instruction,instruction,matching)]", []int{5, 6, 7, 11, 13, 14, 15, 23, 24, 25, 31, 36, 37, 38, 38}, []*string{nil}},
		{7, "[variants(variant(task(instruction,instruction,ordering)),variant(task(instruction,ordering)),variant(task(instruction,instruction,ordering)))]", []int{5, 6, 7, 8, 9, 9, 14, 15, 16, 17, 25, 26, 27, 28, 28}, []*string{nil}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.number), func(t *testing.T) {
			input, problems := parsed.Load(filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", tc.number)))
			if len(problems) != 0 {
				t.Fatal(problems)
			}
			pages, bindings, problems := structure.Build(input, task26Expand(t, input))
			if len(problems) != 0 {
				t.Fatal(problems)
			}
			task26Invariants(t, pages, bindings)
			if got := task26Shape(pages); got != tc.shape {
				t.Fatalf("shape=%s; want %s", got, tc.shape)
			}
			firstLines := make([]int, len(bindings))
			for i, binding := range bindings {
				firstLines[i] = binding.Entries[0].Source.Number
				node := binding.Node(pages)
				context := diagnostic.Context{Path: input.Path, LessonID: *input.Result.Document.Metadata.DocumentID}
				switch node.Type {
				case lesson.NodeMedia:
					// Байты ресурсов принадлежат resourceembed, не structure.
				case lesson.NodeQuestion, lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering:
					if _, _, problem := exercisedata.Build(node.Type, binding.Lines(), context); problem != nil {
						t.Fatalf("consumer %s at %d: %v", node.Type, firstLines[i], problem)
					}
				default:
					declaration, lines := binding.TextSource()
					if _, problem := textdata.Build(node.Type, declaration, lines, context); problem != nil {
						t.Fatalf("consumer %s at %d: %v", node.Type, firstLines[i], problem)
					}
				}
				if node.Type == lesson.NodeTask {
					speaking := tc.number == 3 && firstLines[i] == 6
					stepped := tc.number == 3 && firstLines[i] == 34
					if *node.Flags.Speaking != speaking || *node.Flags.Stepped != stepped {
						t.Fatalf("wrong task flags at %d", firstLines[i])
					}
				}
			}
			if !slices.Equal(firstLines, tc.firstLines) {
				t.Fatalf("preorder source=%v; want %v", firstLines, tc.firstLines)
			}
			if len(pages) != len(tc.titles) {
				t.Fatalf("page count=%d", len(pages))
			}
			for i, want := range tc.titles {
				if want == nil {
					if pages[i].Title != nil {
						t.Fatal("unnamed page got a title")
					}
					continue
				}
				if pages[i].Title == nil || *pages[i].Title != *want {
					t.Fatalf("page %d title changed", i)
				}
			}
			task26Invariants(t, pages, bindings)
		})
	}
}
