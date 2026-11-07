package compare

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func task9Text(value string) lesson.Data {
	return lesson.Data{Type: lesson.DataText, Value: value, Answers: []string{}}
}

func task9Role(role lesson.DataRole) *lesson.DataRole { return &role }

func task9Collection(children ...lesson.Data) lesson.Data {
	if children == nil {
		children = []lesson.Data{}
	}
	return lesson.Data{Type: lesson.DataCollection, Value: children, Answers: []string{}}
}

func task9Media(file lesson.EmbeddedFile) lesson.Data {
	return lesson.Data{Type: lesson.DataMedia, Value: file, Answers: []string{}}
}

// task9Validate проверяет предусловие через настоящий API lesson, помещая
// элементы в обычную коллекцию: здесь допустима отсутствующая роль.
func task9Validate(t *testing.T, values []lesson.Data) {
	t.Helper()
	body := task9Collection(values...)
	body.Role = task9Role(lesson.RoleBody)
	snapshot := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "QA9", Version: "1", LineCount: "0", CreatedAt: "2026-11-06T00:00:00Z",
		Pages: []lesson.Page{{ID: "qa9-p001", Nodes: []lesson.Node{{
			ID: "qa9-n001", Type: lesson.NodeText, Data: []lesson.Data{body}, Nodes: []lesson.Node{},
		}}}},
	}}
	if problems := lesson.Validate(snapshot, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("невалидная тестовая модель: %v", problems)
	}
}

func task9Snapshot(t *testing.T, values []lesson.Data) []byte {
	t.Helper()
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func task9CheckList(t *testing.T, left, right []lesson.Data, want bool) {
	t.Helper()
	task9Validate(t, left)
	task9Validate(t, right)
	beforeLeft, beforeRight := task9Snapshot(t, left), task9Snapshot(t, right)
	if got := equalDataList(left, right); got != want {
		t.Errorf("equalDataList(left, right) = %v, want %v", got, want)
	}
	if got := equalDataList(right, left); got != want {
		t.Errorf("equalDataList(right, left) = %v, want %v", got, want)
	}
	if !equalDataList(left, left) || !equalDataList(right, right) {
		t.Error("нарушена рефлексивность")
	}
	if !bytes.Equal(beforeLeft, task9Snapshot(t, left)) || !bytes.Equal(beforeRight, task9Snapshot(t, right)) {
		t.Error("сравнение изменило входные данные")
	}
}

func TestTask9DataListLengthOrderAndRecursion(t *testing.T) {
	a, b := task9Text("первый"), task9Text("второй")
	cases := []struct {
		name        string
		left, right []lesson.Data
		want        bool
	}{
		{"empty", []lesson.Data{}, []lesson.Data{}, true},
		{"equal independent values", []lesson.Data{a, b}, []lesson.Data{task9Text("первый"), task9Text("второй")}, true},
		{"length", []lesson.Data{a}, []lesson.Data{a, b}, false},
		{"order", []lesson.Data{a, b}, []lesson.Data{b, a}, false},
		{"duplicates count", []lesson.Data{a, a}, []lesson.Data{a}, false},
		{"last element", []lesson.Data{a, a, b}, []lesson.Data{a, a, a}, false},
		{"nested equal", []lesson.Data{task9Collection(a, task9Collection(b))}, []lesson.Data{task9Collection(a, task9Collection(b))}, true},
		{"nested length", []lesson.Data{task9Collection(a, b)}, []lesson.Data{task9Collection(a)}, false},
		{"nested order", []lesson.Data{task9Collection(a, b)}, []lesson.Data{task9Collection(b, a)}, false},
		{"nested shape", []lesson.Data{task9Collection(task9Collection(a), b)}, []lesson.Data{task9Collection(a, task9Collection(b))}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { task9CheckList(t, tc.left, tc.right, tc.want) })
	}
}

func TestTask9DataAllOwnFields(t *testing.T) {
	base := func() lesson.Data {
		d := task9Text("Ответ")
		d.Role, d.Answers, d.Hint = task9Role(lesson.RoleBody), []string{"да", "нет", "да"}, "подсказка"
		return d
	}
	cases := []struct {
		name   string
		change func(*lesson.Data)
		want   bool
	}{
		{"independent role pointer", func(d *lesson.Data) { d.Role = task9Role(lesson.RoleBody) }, true},
		{"role absence", func(d *lesson.Data) { d.Role = nil }, false},
		{"role value", func(d *lesson.Data) { d.Role = task9Role(lesson.RoleScript) }, false},
		{"type", func(d *lesson.Data) { d.Type, d.Value = lesson.DataCollection, []lesson.Data{} }, false},
		{"value", func(d *lesson.Data) { d.Value = "Другой ответ" }, false},
		{"answers order", func(d *lesson.Data) { d.Answers = []string{"нет", "да", "да"} }, false},
		{"answers length", func(d *lesson.Data) { d.Answers = []string{"да", "нет"} }, false},
		{"answers exact", func(d *lesson.Data) { d.Answers = []string{"да", "Нет", "да"} }, false},
		{"answers spaces", func(d *lesson.Data) { d.Answers = []string{"да ", "нет", "да"} }, false},
		{"answers newline", func(d *lesson.Data) { d.Answers = []string{"да\n", "нет", "да"} }, false},
		{"answers html", func(d *lesson.Data) { d.Answers = []string{"<b>да</b>", "нет", "да"} }, false},
		{"hint", func(d *lesson.Data) { d.Hint = "Подсказка" }, false},
		{"hint spaces", func(d *lesson.Data) { d.Hint += " " }, false},
		{"hint newline", func(d *lesson.Data) { d.Hint += "\n" }, false},
		{"hint html", func(d *lesson.Data) { d.Hint = "<b>подсказка</b>" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left, right := base(), base()
			tc.change(&right)
			task9CheckList(t, []lesson.Data{left}, []lesson.Data{right}, tc.want)
			task9CheckList(t, []lesson.Data{task9Collection(left)}, []lesson.Data{task9Collection(right)}, tc.want)
			if got := equalData(left, right); got != tc.want {
				t.Errorf("equalData = %v, want %v", got, tc.want)
			}
		})
	}
	t.Run("both roles absent", func(t *testing.T) {
		task9CheckList(t, []lesson.Data{task9Text("")}, []lesson.Data{task9Text("")}, true)
	})
}

func TestTask9TextIsOpaque(t *testing.T) {
	cases := [][2]string{
		{" text", "text"}, {"text ", "text"}, {"a  b", "a b"}, {"A", "a"},
		{"a\nb", "a\r\nb"}, {"a\n\nb", "a\nb"}, {"a\n", "a"},
		{"<b>x</b>", "x"}, {"&amp;", "&"}, {"<B>x</B>", "<b>x</b>"},
		{"_____{ANSWER}", "_____{answer}"}, {"_____{one;two}", "_____{two;one}"},
		{"до\n\n_____{ДА}\nпосле", "до\n_____{ДА}\nпосле"}, {"é", "e\u0301"},
	}
	for _, pair := range cases {
		t.Run(pair[0], func(t *testing.T) {
			left, right := task9Text(pair[0]), task9Text(pair[1])
			task9CheckList(t, []lesson.Data{left}, []lesson.Data{right}, false)
			if equalDataValue(left, right) {
				t.Error("equalDataValue нормализовал непрозрачный текст")
			}
		})
	}
}

func TestTask9DataValueValidatedUnion(t *testing.T) {
	file := lesson.EmbeddedFile{FileName: "файл.bin", MIMEType: "application/octet-stream", Content: "AP8="}
	values := []lesson.Data{task9Text(""), task9Text("a\n\n_____{ANSWER}"), task9Media(file), task9Collection(), task9Collection(task9Text("a"))}
	for i, left := range values {
		task9Validate(t, []lesson.Data{left})
		if !equalDataValue(left, left) {
			t.Errorf("union variant %d unequal to itself", i)
		}
	}
	left, right := task9Collection(task9Text("a")), task9Collection(task9Text("b"))
	task9Validate(t, []lesson.Data{left, right})
	if equalDataValue(left, right) {
		t.Error("equalDataValue пропустил отличие дочернего значения")
	}
	left, right = task9Text("одинаковое значение"), task9Text("одинаковое значение")
	right.Role, right.Answers, right.Hint = task9Role(lesson.RoleTitle), []string{"ответ"}, "подсказка"
	task9Validate(t, []lesson.Data{left, right})
	if !equalDataValue(left, right) {
		t.Error("equalDataValue сравнил поля вне Value")
	}
}

func TestTask9EmbeddedFileExactMetadataAndBytes(t *testing.T) {
	file := func(name, mime string, raw []byte) lesson.EmbeddedFile {
		return lesson.EmbeddedFile{FileName: name, MIMEType: mime, Content: base64.StdEncoding.EncodeToString(raw)}
	}
	base := file("данные.bin", "application/octet-stream", []byte{0, 255, 128, 13, 10, 0})
	cases := []struct {
		name  string
		right lesson.EmbeddedFile
		want  bool
	}{
		{"same binary", file("данные.bin", "application/octet-stream", []byte{0, 255, 128, 13, 10, 0}), true},
		{"file name", file("Данные.bin", base.MIMEType, []byte{0, 255, 128, 13, 10, 0}), false},
		{"file name spaces", file("данные.bin ", base.MIMEType, []byte{0, 255, 128, 13, 10, 0}), false},
		{"mime case", file(base.FileName, "Application/octet-stream", []byte{0, 255, 128, 13, 10, 0}), false},
		{"mime spaces", file(base.FileName, base.MIMEType+" ", []byte{0, 255, 128, 13, 10, 0}), false},
		{"byte difference", file(base.FileName, base.MIMEType, []byte{0, 255, 129, 13, 10, 0}), false},
		{"byte length", file(base.FileName, base.MIMEType, []byte{0, 255, 128, 13, 10}), false},
		{"empty versus binary", file(base.FileName, base.MIMEType, []byte{}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task9CheckList(t, []lesson.Data{task9Media(base)}, []lesson.Data{task9Media(tc.right)}, tc.want)
			if equalEmbeddedFile(base, tc.right) != tc.want || equalEmbeddedFile(tc.right, base) != tc.want {
				t.Error("equalEmbeddedFile: неверное равенство")
			}
			if equalDataValue(task9Media(base), task9Media(tc.right)) != tc.want {
				t.Error("equalDataValue: неверное равенство файлов")
			}
		})
	}
	for _, raw := range [][]byte{{}, {0}, {255}, {0, 1}, {0, 1, 2}, {0, 255, 128, 13, 10}} {
		encoded := base64.StdEncoding.EncodeToString(raw)
		if !equalEmbeddedContent(encoded, encoded) {
			t.Error("не равны одинаковые байты, включая пустой файл")
		}
	}
	if equalEmbeddedContent("AA==", "/w==") || equalEmbeddedContent("", "AA==") {
		t.Error("равны различные декодированные байты")
	}
	empty := lesson.EmbeddedFile{FileName: "пустой.bin", MIMEType: "application/octet-stream"}
	task9CheckList(t, []lesson.Data{task9Media(empty)}, []lesson.Data{task9Media(empty)}, true)
}

func TestTask9DataDeepAndLarge(t *testing.T) {
	left, right, changed := task9Text("конец"), task9Text("конец"), task9Text("изменён")
	for range 512 {
		left, right, changed = task9Collection(left), task9Collection(right), task9Collection(changed)
	}
	task9CheckList(t, []lesson.Data{left}, []lesson.Data{right}, true)
	task9CheckList(t, []lesson.Data{left}, []lesson.Data{changed}, false)
	long := strings.Repeat("я\n_____{ДА}\n\n", 1<<15)
	task9CheckList(t, []lesson.Data{task9Text(long)}, []lesson.Data{task9Text(long)}, true)
	task9CheckList(t, []lesson.Data{task9Text(long)}, []lesson.Data{task9Text(long + "!")}, false)
	wideLeft, wideRight := make([]lesson.Data, 4096), make([]lesson.Data, 4096)
	for i := range wideLeft {
		wideLeft[i], wideRight[i] = task9Text("элемент"), task9Text("элемент")
	}
	task9CheckList(t, wideLeft, wideRight, true)
	wideRight[len(wideRight)-1] = task9Text("последний изменён")
	task9CheckList(t, wideLeft, wideRight, false)
}
