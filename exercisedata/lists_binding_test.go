package exercisedata

import (
	"testing"

	"github.com/verdoga/dsl-builder/parsed/model"
)

func exListBindingEmptyAnswer(t *testing.T, form string, number int) []model.Line {
	t.Helper()
	switch form {
	case "empty tag":
		return []model.Line{exListLine(number, 100, model.LineTypeTag, "answer", "")}
	case "whitespace tag":
		return []model.Line{exListLine(number, 100, model.LineTypeTag, "answer", " \t")}
	case "empty block":
		return []model.Line{
			exListLine(number, 100, model.LineTypeBlockStart, "answer", ""),
			exListLine(number+1, number, model.LineTypeBlockEnd, "", ""),
		}
	default:
		t.Fatalf("unknown answer form %q", form)
		return nil
	}
}

func exListBindingIndent(lines []model.Line, indent string) {
	shift := len([]rune(indent))
	for i := range lines {
		lines[i].Raw = indent + lines[i].Raw
		for j := range lines[i].Elements {
			lines[i].Elements[j].Start += shift
			lines[i].Elements[j].End += shift
		}
	}
}

func TestExListBindingRepeatedEmptyAnswerRetainsParticipants(t *testing.T) {
	for _, form := range []string{"empty tag", "whitespace tag", "empty block"} {
		for _, indent := range []string{"", " \t"} {
			t.Run(form+"/indent="+indent, func(t *testing.T) {
				body := []model.Line{
					exListLine(101, 100, model.LineTypeContent, "", "item"),
					exListLine(102, 100, model.LineTypeTag, "answer", "A"),
					exListLine(103, 100, model.LineTypeTag, "hint", "first"),
				}
				body = append(body, exListBindingEmptyAnswer(t, form, 104)...)
				exListBindingIndent(body, indent)
				lines := exListFrame("matching", body...)
				before := exListJSON(t, lines)
				_, err := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
				exListError(t, err, CodeBinding)
				column := len([]rune(indent)) + 1
				if err.Context != exListContext(104, column) {
					t.Fatalf("repeated answer source = %+v; want tag 104:%d", err.Context, column)
				}
				if len(err.Related) != 4 {
					t.Fatalf("participants = %+v; want item, first answer, linked hint, repeated answer", err.Related)
				}
				for i, participant := range err.Related {
					if participant != exListContext(101+i, column) {
						t.Fatalf("participant %d = %+v; want source %d:%d", i, participant, 101+i, column)
					}
				}
				diagnosticBefore := exListJSON(t, err)
				_, second := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
				if exListJSON(t, second) != diagnosticBefore {
					t.Fatal("conflict diagnostic changes between calls")
				}
				err.Related[0].Line = 999
				err.Related[3].Column = 999
				_, fresh := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
				if exListJSON(t, second) != diagnosticBefore || exListJSON(t, fresh) != diagnosticBefore || exListJSON(t, lines) != before {
					t.Fatal("conflict diagnostic aliases another result or mutates input")
				}
			})
		}
	}
}

func TestExListBindingFirstEmptyAnswerRemainsAnswerError(t *testing.T) {
	for _, form := range []string{"empty tag", "whitespace tag", "empty block"} {
		t.Run(form, func(t *testing.T) {
			body := []model.Line{exListLine(101, 100, model.LineTypeContent, "", "item")}
			body = append(body, exListBindingEmptyAnswer(t, form, 102)...)
			lines := exListFrame("matching", body...)
			before := exListJSON(t, lines)
			_, err := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
			exListError(t, err, CodeAnswer)
			_, again := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
			if exListJSON(t, again) != exListJSON(t, err) || exListJSON(t, lines) != before {
				t.Fatal("first empty answer changes diagnostic or mutates input")
			}
		})
	}
}
