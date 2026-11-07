package catalog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

func editTestRequire(t *testing.T, ok bool, message string) {
	t.Helper()
	if !ok {
		t.Fatal(message)
	}
}

func editTestPtr(s string) *string { return &s }

func editTestState() *State {
	s := &State{CourseTitle: "Курс", UnitTitle: editTestPtr("Юнит")}
	for i, name := range []string{"l001.json", "l002.json", "l003.json"} {
		s.Items = append(s.Items, Item{Entry: manifest.Entry{ID: "ID-" + name, FileName: name,
			Order: editTestPtr([]string{"1", "2", "3"}[i]), Title: "Урок " + name,
			Subtitle: editTestPtr("Подзаголовок"), LineCount: json.Number("9007199254740993"),
			Version: json.Number("12345678901234567890"), CreatedAt: "2026-11-04T09:00:00Z"},
			RelativePath: "вход/" + name, Context: diagnostic.Context{Path: "source/" + name, FileName: name, LessonID: "ID-" + name, Line: i + 1}})
	}
	return s
}

func editTestSnapshot(t *testing.T, s *State) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func editTestCode(t *testing.T, errs []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, err := range errs {
		if err != nil && err.Code == code {
			return err
		}
	}
	t.Fatalf("нет кода %s: %v", code, errs)
	return nil
}

func editTestItem(t *testing.T, s *State, name string) Item {
	t.Helper()
	for _, item := range s.Items {
		if item.Entry.FileName == name {
			return item
		}
	}
	t.Fatalf("нет урока %s", name)
	return Item{}
}

func TestEditNilStateAndInvalidInputs(t *testing.T) {
	for name, fn := range map[string]func(*State) (*State, []*diagnostic.Error){
		"course":  func(s *State) (*State, []*diagnostic.Error) { return SetCourseTitle(s, "Курс") },
		"unit":    func(s *State) (*State, []*diagnostic.Error) { return SetUnitTitle(s, "") },
		"section": func(s *State) (*State, []*diagnostic.Error) { return CreateSection(s, "Секция") },
		"move":    func(s *State) (*State, []*diagnostic.Error) { return MoveLesson(s, "l001.json", nil) },
		"flat":    UseFlatPlacement,
		"weight":  func(s *State) (*State, []*diagnostic.Error) { return SetLessonWeight(s, "l001.json", "4") },
	} {
		t.Run(name, func(t *testing.T) {
			got, errs := fn(nil)
			editTestRequire(t, got == nil, "ожидался nil")
			editTestCode(t, errs, CodeState)
		})
	}
	s := editTestState()
	before := editTestSnapshot(t, s)
	for _, title := range []string{"", " \t\r\n\u00a0", string([]byte{0xff})} {
		got, errs := SetCourseTitle(s, title)
		editTestRequire(t, got == nil, "неверный курс применён")
		editTestCode(t, errs, manifest.CodeTitle)
		if title != "" {
			got, errs = SetUnitTitle(s, title)
			editTestRequire(t, got == nil, "неверный юнит применён")
			editTestCode(t, errs, manifest.CodeTitle)
		}
	}
	editTestRequire(t, !(editTestSnapshot(t, s) != before), "неверный ввод изменил источник")
}

func TestEditTitlesPreservePlacementAndOwnership(t *testing.T) {
	s := editTestState()
	s.Sectioned = true
	s.Sections = []SectionOrder{{Title: "Z", Weight: editTestPtr("007")}}
	for i := range s.Items {
		s.Items[i].Section = editTestPtr("Z")
	}
	before := editTestSnapshot(t, s)
	a, errs := SetCourseTitle(s, "  Новый\tкурс  ")
	if a == nil || len(errs) != 0 {
		t.Fatalf("%v", errs)
	}
	b, errs := SetUnitTitle(s, "")
	if b == nil || len(errs) != 0 || b.UnitTitle != nil {
		t.Fatalf("%v", errs)
	}
	c, errs := SetUnitTitle(s, " Новый юнит ")
	if c == nil || len(errs) != 0 || *c.UnitTitle != " Новый юнит " {
		t.Fatalf("%v", errs)
	}
	editTestRequire(t, !(a.CourseTitle != "  Новый\tкурс  " || a.Sections[0].Title != "Z" || len(a.Items) != 3), "название/размещение изменено")
	a.CourseTitle = s.CourseTitle
	editTestRequire(t, !(editTestSnapshot(t, a) != before), "изменены посторонние данные курса")
	b.UnitTitle = editTestPtr("Юнит")
	editTestRequire(t, !(editTestSnapshot(t, b) != before), "изменены посторонние данные юнита")
	*a.UnitTitle = "изменён"
	*a.Items[0].Entry.Order = "99"
	*a.Items[0].Entry.Subtitle = "иной"
	*a.Items[0].Section = "иная"
	*a.Sections[0].Weight = "9"
	a.Items[0].Context.Path = "иной"
	editTestRequire(t, !(editTestSnapshot(t, s) != before || editTestSnapshot(t, b) != before), "кандидаты разделяют изменяемые данные")
}

func TestEditNormalizeSectionTitleOnlySpecifiedCharacters(t *testing.T) {
	ctx := diagnostic.Context{Path: "ввод", Line: 4}
	for _, pair := range [][2]string{
		{" \tA\r\n  B\t ", "A B"}, {"Studentbook", "Studentbook"},
		{"\u00a0A\u00a0\u00a0B\u00a0", "\u00a0A\u00a0\u00a0B\u00a0"},
		{"\vA\f\u2028B\u3000", "\vA\f\u2028B\u3000"}, {"E\u0301  É", "E\u0301 É"},
	} {
		got, err := NormalizeSectionTitle(pair[0], ctx)
		if err != nil || got != pair[1] {
			t.Errorf("%q => %q, %v", pair[0], got, err)
		}
	}
	for _, raw := range []string{"", " \t\r\n ", "\u00a0\u3000", string([]byte{0xff})} {
		_, err := NormalizeSectionTitle(raw, ctx)
		if err == nil || err.Code != manifest.CodeTitle || err.Context != ctx {
			t.Errorf("%q: %v", raw, err)
		}
	}
}

func TestEditCreateSectionIsExactAndIncompleteCandidate(t *testing.T) {
	s := editTestState()
	before := editTestSnapshot(t, s)
	a, errs := CreateSection(s, "Studentbook")
	if a == nil || !a.Sectioned || len(a.Sections) != 1 || a.Sections[0].Title != "Studentbook" {
		t.Fatalf("%+v %v", a, errs)
	}
	editTestCode(t, errs, CodeUnassigned)
	for _, item := range a.Items {
		editTestRequire(t, item.Section == nil, "автоматическое распределение")
	}
	if out, errs := Export(a, "2026-11-04T09:00:00Z", diagnostic.Context{}); out != nil || len(errs) == 0 {
		t.Fatal("незавершённый план экспортирован")
	}
	b, _ := CreateSection(a, "Studentbook")
	editTestRequire(t, !(b == nil || len(b.Sections) != 1), "точное имя продублировано")
	c, _ := CreateSection(b, "studentbook")
	editTestRequire(t, !(c == nil || len(c.Sections) != 2 || c.Sections[1].Title != "studentbook"), "регистр потерян/порядок не сохранён")
	for _, raw := range []string{"", " New ", "A  B", "A\tB"} {
		got, errs := CreateSection(s, raw)
		if got != nil {
			t.Fatalf("ввод %q скрыто нормализован", raw)
		}
		editTestCode(t, errs, manifest.CodeTitle)
	}
	for _, item := range a.Items {
		c, errs = MoveLesson(c, item.Entry.FileName, editTestPtr("Studentbook"))
		if c == nil {
			t.Fatal(errs)
		}
	}
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if out, errs := Export(c, "2026-11-04T09:00:00Z", diagnostic.Context{}); out == nil || len(errs) != 0 {
		t.Fatal(errs)
	}
	editTestRequire(t, !(editTestSnapshot(t, s) != before), "источник изменён")
}

func TestEditMoveLessonPreservesMetadataAndChecksWholeModule(t *testing.T) {
	s := editTestState()
	s.Sectioned = true
	s.Sections = []SectionOrder{{Title: "B"}, {Title: "A"}, {Title: "C"}}
	s.Items[0].Section, s.Items[1].Section, s.Items[2].Section = editTestPtr("A"), editTestPtr("B"), editTestPtr("B")
	s.Items[0].Entry.Order, s.Items[1].Entry.Order, s.Items[2].Entry.Order = editTestPtr("001"), editTestPtr("1"), editTestPtr("01")
	for _, name := range []string{"l004.json", "l005.json"} {
		item := editTestState().Items[0]
		item.Entry.FileName = name
		item.Context.FileName = name
		item.Section = editTestPtr("C")
		item.Entry.Order = editTestPtr("9")
		s.Items = append(s.Items, item)
	}
	before := editTestSnapshot(t, s)
	section := "B"
	got, errs := MoveLesson(s, "l001.json", &section)
	if got == nil {
		t.Fatal(errs)
	}
	original := s.Items[0]
	moved := editTestItem(t, got, "l001.json")
	moved.Section = original.Section
	if editTestSnapshot(t, &State{Items: []Item{moved}}) != editTestSnapshot(t, &State{Items: []Item{original}}) {
		t.Fatal("перенос изменил метаданные/вес/имя")
	}
	counts := []int{}
	for _, err := range errs {
		if err.Code == CodeWeightConflict {
			counts = append(counts, len(err.Related))
		}
	}
	if len(counts) != 2 || counts[0] != 3 || counts[1] != 2 {
		t.Fatalf("не все участники/группы: %v", counts)
	}
	section = "изменено"
	editTestRequire(t, !(*editTestItem(t, got, "l001.json").Section != "B"), "входной указатель сохранён")
	fresh, _ := MoveLesson(s, "l001.json", editTestPtr("New"))
	editTestRequire(t, !(fresh == nil || len(fresh.Sections) != 4 || fresh.Sections[3].Title != "New"), "новая секция не добавлена в конец")
	detached, errs := MoveLesson(s, "l001.json", nil)
	if detached == nil || editTestItem(t, detached, "l001.json").Section != nil {
		t.Fatal(errs)
	}
	editTestCode(t, errs, CodeUnassigned)
	for _, selector := range []string{"ID-l001.json", "missing"} {
		bad, errs := MoveLesson(s, selector, nil)
		editTestRequire(t, bad == nil, "неверный селектор применён")
		editTestCode(t, errs, CodeSelection)
	}
	editTestRequire(t, !(editTestSnapshot(t, s) != before), "источник изменён")
}

func TestEditFlatPlacementRequiresExplicitRemoval(t *testing.T) {
	s := editTestState()
	s.Sectioned = true
	s.Sections = []SectionOrder{{Title: "B"}, {Title: "A"}}
	s.Items[0].Section, s.Items[1].Section, s.Items[2].Section = editTestPtr("A"), editTestPtr("B"), editTestPtr("B")
	before := editTestSnapshot(t, s)
	got, errs := UseFlatPlacement(s)
	editTestRequire(t, !(got != nil || len(errs) == 0), "назначения сняты автоматически")
	editTestRequire(t, !(editTestSnapshot(t, s) != before), "отказ изменил источник")
	for i := range s.Items {
		s.Items[i].Section = nil
	}
	s.Items[1].Entry.Order = editTestPtr("01")
	got, errs = UseFlatPlacement(s)
	editTestRequire(t, !(got == nil || got.Sectioned || len(got.Sections) != 0), "пустые рабочие секции не сняты")
	editTestCode(t, errs, CodeWeightConflict)
	editTestRequire(t, !(!s.Sectioned || len(s.Sections) != 2), "источник принят без подтверждения")
}

func TestEditWeightRawErrorsAndCandidateIsolation(t *testing.T) {
	s := editTestState()
	before := editTestSnapshot(t, s)
	for _, raw := range []string{"", "0", "000", "-1", "+1", "1.0", "1e2", " 1", "1\n", "１２"} {
		got, errs := SetLessonWeight(s, "l001.json", raw)
		if got != nil {
			t.Fatalf("применён %q", raw)
		}
		err := editTestCode(t, errs, order.CodeInvalidWeight)
		editTestRequire(t, !(err.Context != s.Items[0].Context), "потерян контекст")
		editTestRequire(t, !(len(err.Details) == 0 || err.Details[0].Value != raw), "потерян исходный вес")
	}
	for _, raw := range []string{"0002", "000" + strings.Repeat("9", 1000)} {
		got, errs := SetLessonWeight(s, "l001.json", raw)
		if got == nil {
			t.Fatal(errs)
		}
		editTestRequire(t, !(*editTestItem(t, got, "l001.json").Entry.Order != raw), "вес нормализован/округлён")
		if raw == "0002" {
			editTestCode(t, errs, CodeWeightConflict)
		} else if len(errs) != 0 {
			t.Fatal(errs)
		}
	}
	a, errs := SetLessonWeight(s, "l001.json", "4")
	editTestRequire(t, !(a == nil || len(errs) != 0 || a.Items[2].Entry.FileName != "l001.json"), "Refresh не сортирует кандидата")
	b, _ := SetLessonWeight(s, "l001.json", "5")
	*editTestItem(t, a, "l001.json").Entry.Subtitle = "изменён"
	editTestRequire(t, !(editTestSnapshot(t, s) != before || *editTestItem(t, b, "l001.json").Entry.Subtitle != "Подзаголовок"), "отказ/снимки не независимы")
	duplicate := editTestState()
	duplicate.Items = append(duplicate.Items, duplicate.Items[0])
	bad, errs := SetLessonWeight(duplicate, "l001.json", "4")
	editTestRequire(t, bad == nil, "неоднозначный селектор")
	editTestCode(t, errs, CodeSelection)
	s.Items[1].Entry.Order = nil
	a, errs = SetCourseTitle(s, "Другой курс")
	if a == nil {
		t.Fatal(errs)
	}
	editTestCode(t, errs, CodeMissingWeight)
}
