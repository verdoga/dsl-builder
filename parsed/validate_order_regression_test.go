package parsed

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/order"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestValidateOrderRegressionLineFieldBeforeNumberValue(t *testing.T) {
	result := validateOrderRegressionTestResult("0", model.LineType("unknown"),
		validateOrderRegressionTestElement(model.ElementTypeNumber, "0", "0", 1, 2))
	lineOnly := validateOrderRegressionTestClone(t, result)
	*lineOnly.Lines[0].Elements[0].Value = "1"
	numberOnly := validateOrderRegressionTestClone(t, result)
	numberOnly.Lines[0].LineType = model.LineTypeContent
	validateOrderRegressionTestRun(t, result, []model.Result{lineOnly, numberOnly},
		[]string{CodeContract, order.CodeInvalidWeight}, []int{0, 1})
}

func TestValidateOrderRegressionElementIndexBeforeNextNumberValue(t *testing.T) {
	result := validateOrderRegressionTestResult("x0", model.LineTypeContent,
		validateOrderRegressionTestElement(model.ElementType("unknown"), "x", "x", 1, 2),
		validateOrderRegressionTestElement(model.ElementTypeNumber, "0", "0", 2, 3))
	firstOnly := validateOrderRegressionTestClone(t, result)
	*firstOnly.Lines[0].Elements[1].Value = "1"
	secondOnly := validateOrderRegressionTestClone(t, result)
	secondOnly.Lines[0].Elements[0].ElementType = model.ElementTypeContent
	validateOrderRegressionTestRun(t, result, []model.Result{firstOnly, secondOnly},
		[]string{CodeContract, order.CodeInvalidWeight}, []int{1, 2})
}

func TestValidateOrderRegressionSameStartKeepsElementIndices(t *testing.T) {
	// Пустой [1,1) предшествует [1,2): равные start не отождествляют элементы.
	result := validateOrderRegressionTestResult("x", model.LineTypeContent,
		validateOrderRegressionTestElement(model.ElementTypeBlockOpen, "", "{", 1, 1),
		validateOrderRegressionTestElement(model.ElementType("unknown"), "x", "x", 1, 2))
	firstOnly := validateOrderRegressionTestClone(t, result)
	firstOnly.Lines[0].Elements[1].ElementType = model.ElementTypeContent
	secondOnly := validateOrderRegressionTestClone(t, result)
	secondOnly.Lines[0].Elements[0].ElementType = model.ElementTypeContent
	*secondOnly.Lines[0].Elements[0].Value = ""
	validateOrderRegressionTestRun(t, result, []model.Result{firstOnly, secondOnly},
		[]string{CodeContract, CodeContract}, []int{1, 1})
}

// validateOrderRegressionTestRun сравнивает общий список с независимыми
// одиночными ошибками в нормативном порядке без литералов Reason и Label.
func validateOrderRegressionTestRun(t *testing.T, result model.Result, isolated []model.Result, codes []string, columns []int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "порядок diagnostics.json")
	context := diagnostic.Context{Path: path, LessonID: *result.Document.Metadata.DocumentID}
	want := make([]*diagnostic.Error, len(isolated))
	for i := range isolated {
		_, problems := validateContract(&isolated[i], context)
		if len(problems) != 1 || problems[0] == nil {
			t.Fatalf("контроль %d должен содержать ровно одну ошибку: %+v", i, problems)
		}
		want[i] = problems[0]
		expectedContext := context
		expectedContext.Line, expectedContext.Column = 1, columns[i]
		if want[i].Code != codes[i] || want[i].Context != expectedContext {
			t.Fatalf("контроль %d: Code/Context = %s/%+v, ожидались %s/%+v", i,
				want[i].Code, want[i].Context, codes[i], expectedContext)
		}
		if want[i].Code == order.CodeInvalidWeight {
			dependency := order.Validate("0", expectedContext)
			if dependency == nil || want[i].Code != dependency.Code || want[i].Context != dependency.Context {
				t.Fatalf("контроль %d потерял Code/Context настоящего order.Validate", i)
			}
			next := 0
			for _, detail := range want[i].Details {
				if next < len(dependency.Details) && detail == dependency.Details[next] {
					next++
				}
			}
			if next != len(dependency.Details) {
				t.Fatalf("контроль %d потерял подробности order.Validate: %+v", i, want[i].Details)
			}
		}
	}
	if bytes.Equal(validateOrderRegressionTestSignature(t, want[:1]), validateOrderRegressionTestSignature(t, want[1:])) {
		t.Fatal("контрольные ошибки должны различаться структурированными сведениями")
	}
	wantSignature := validateOrderRegressionTestSignature(t, want)
	before := validateOrderRegressionTestJSON(t, result)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("validateContract", func(t *testing.T) {
		var previous []byte
		for attempt := 0; attempt < 12; attempt++ {
			index, problems := validateContract(&result, context)
			if index.ByNumber != nil || index.Children != nil {
				t.Errorf("попытка %d вернула частичный индекс: %+v", attempt, index)
			}
			validateOrderRegressionTestCheck(t, problems, wantSignature)
			current := validateOrderRegressionTestJSON(t, problems)
			if attempt > 0 && !bytes.Equal(current, previous) {
				t.Errorf("попытка %d изменила диагностику", attempt)
			}
			previous = current
			if !bytes.Equal(before, validateOrderRegressionTestJSON(t, result)) {
				t.Fatalf("попытка %d изменила входную модель", attempt)
			}
		}
	})
	t.Run("Load", func(t *testing.T) {
		var previous []byte
		for attempt := 0; attempt < 12; attempt++ {
			input, problems := Load(path)
			if input != nil {
				t.Errorf("попытка %d вернула пригодный частичный Input", attempt)
			}
			validateOrderRegressionTestCheck(t, problems, wantSignature)
			current := validateOrderRegressionTestJSON(t, problems)
			if attempt > 0 && !bytes.Equal(current, previous) {
				t.Errorf("попытка %d изменила диагностику Load", attempt)
			}
			previous = current
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("попытка %d изменила исходный JSON", attempt)
			}
		}
	})
}

// validateOrderRegressionTestCheck проверяет точный порядок Code/Context/Details.
func validateOrderRegressionTestCheck(t *testing.T, problems []*diagnostic.Error, want []byte) {
	t.Helper()
	got := validateOrderRegressionTestSignature(t, problems)
	if !bytes.Equal(got, want) {
		t.Errorf("нарушен порядок или состав диагностики\nполучено: %s\nожидалось: %s", got, want)
	}
}

// validateOrderRegressionTestSignature сохраняет структурированные сведения;
// свободный текст причины и названия подробностей тест не задаёт литералами.
func validateOrderRegressionTestSignature(t *testing.T, problems []*diagnostic.Error) []byte {
	t.Helper()
	type item struct {
		Code    string
		Context diagnostic.Context
		Related []diagnostic.Context
		Details []diagnostic.Detail
	}
	signature := make([]item, len(problems))
	for i, problem := range problems {
		if problem == nil {
			t.Fatalf("диагностика %d равна nil", i)
		}
		signature[i] = item{problem.Code, problem.Context, problem.Related, problem.Details}
	}
	return validateOrderRegressionTestJSON(t, signature)
}

// validateOrderRegressionTestResult создаёт полную минимальную модель Contract.
func validateOrderRegressionTestResult(raw string, lineType model.LineType, elements ...model.Element) model.Result {
	id := "Review"
	return model.Result{
		Format: model.FormatVersion1,
		Document: model.Document{Metadata: model.DocumentMetadata{
			DocumentID: &id, ResourceDirs: []string{},
		}},
		Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Lines: []model.Line{{Number: 1, LineType: lineType, Raw: raw,
			LineEnding: model.LineEndingNone, Elements: elements}},
	}
}

// validateOrderRegressionTestElement сохраняет непустой указатель value и errorIds=[].
func validateOrderRegressionTestElement(kind model.ElementType, raw, value string, start, end int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: end, ErrorIDs: []string{}}
}

// validateOrderRegressionTestClone создаёт независимую модель, включая указатели.
func validateOrderRegressionTestClone(t *testing.T, result model.Result) model.Result {
	t.Helper()
	var clone model.Result
	if err := json.Unmarshal(validateOrderRegressionTestJSON(t, result), &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

// validateOrderRegressionTestJSON сериализует полное наблюдаемое значение.
func validateOrderRegressionTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
