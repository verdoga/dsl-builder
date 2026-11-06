package lesson

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// UnmarshalJSON координирует checkLessonJSON и декодирование известных полей
// через decodeDocumentJSON. Получатель меняется только при успехе.
// Имена свойств сравниваются точно по схеме, включая регистр: постороннее
// TITLE не заменяет title и не затеняет его при наличии обоих ключей.
// Ошибка возвращается как *diagnostic.Error с JSON Pointer; внешний контекст
// дополняет ValidateJSON или moduleio. Семантические ограничения проверяет Validate.
// Метод нужен для сохранения обязательности полей и точного контракта
// при использовании стандартного encoding/json, а не для файлового чтения.
func (value *Lesson) UnmarshalJSON(raw []byte) error {
	context := diagnostic.Context{}
	if problems := checkLessonJSON(raw, context); len(problems) != 0 {
		return problems[0]
	}
	if value == nil {
		return contractError(CodeJSON, "Не задан получатель урока", "", context, nil)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return contractError(CodeJSON, "Не удалось прочитать объект урока", "", context, err)
	}
	var decoded Lesson
	if err := json.Unmarshal(fields["formatVersion"], &decoded.FormatVersion); err != nil {
		return contractError(CodeJSON, "Не удалось прочитать версию формата", "/formatVersion", context, err)
	}
	document, problem := decodeDocumentJSON(fields["document"])
	if problem != nil {
		copied := *problem
		copied.Details = append([]diagnostic.Detail(nil), problem.Details...)
		for i := range copied.Details {
			if copied.Details[i].Label == "Поле JSON" {
				copied.Details[i].Value = "/document" + copied.Details[i].Value
			}
		}
		return &copied
	}
	decoded.Document = document
	*value = decoded
	return nil
}

// decodeDocumentJSON восстанавливает только точные известные поля проверенного
// объекта Document, сохраняя числа как json.Number; страницы делегирует
// decodePageJSON. Незнакомые свойства не переносятся в модель.
func decodeDocumentJSON(raw json.RawMessage) (Document, *diagnostic.Error) {
	var decoded Document
	context := diagnostic.Context{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Document{}, contractError(CodeJSON, "Не удалось прочитать документ", "", context, err)
	}
	for _, field := range []struct {
		name   string
		target any
	}{
		{"id", &decoded.ID}, {"version", &decoded.Version}, {"title", &decoded.Title},
		{"subtitle", &decoded.Subtitle}, {"lineCount", &decoded.LineCount}, {"createdAt", &decoded.CreatedAt},
	} {
		if _, present := fields[field.name]; field.name == "subtitle" && !present {
			continue
		}
		if err := json.Unmarshal(fields[field.name], field.target); err != nil {
			return Document{}, contractError(CodeJSON, "Не удалось прочитать поле документа", "/"+field.name, context, err)
		}
		if field.name == "id" {
			context.LessonID = decoded.ID
		}
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(fields["pages"], &pages); err != nil {
		return Document{}, contractError(CodeJSON, "Не удалось прочитать страницы", "/pages", context, err)
	}
	decoded.Pages = make([]Page, len(pages))
	for i, rawPage := range pages {
		page, problem := decodePageJSON(rawPage)
		if problem != nil {
			copied := *problem
			copied.Context.LessonID = decoded.ID
			copied.Details = append([]diagnostic.Detail(nil), problem.Details...)
			for j := range copied.Details {
				if copied.Details[j].Label == "Поле JSON" {
					copied.Details[j].Value = "/pages/" + strconv.Itoa(i) + copied.Details[j].Value
				}
			}
			return Document{}, &copied
		}
		decoded.Pages[i] = page
	}
	return decoded, nil
}

// decodePageJSON восстанавливает поля проверенной страницы и null названия;
// ноды передаёт decodeNodeJSON. Отсутствующие поля не синтезирует.
func decodePageJSON(raw json.RawMessage) (Page, *diagnostic.Error) {
	var decoded Page
	context := diagnostic.Context{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Page{}, contractError(CodeJSON, "Не удалось прочитать страницу", "", context, err)
	}
	if err := json.Unmarshal(fields["id"], &decoded.ID); err != nil {
		return Page{}, contractError(CodeJSON, "Не удалось прочитать ID страницы", "/id", context, err)
	}
	context.PageID = decoded.ID
	if err := json.Unmarshal(fields["title"], &decoded.Title); err != nil {
		return Page{}, contractError(CodeJSON, "Не удалось прочитать название страницы", "/title", context, err)
	}
	var nodes []json.RawMessage
	if err := json.Unmarshal(fields["nodes"], &nodes); err != nil {
		return Page{}, contractError(CodeJSON, "Не удалось прочитать ноды страницы", "/nodes", context, err)
	}
	decoded.Nodes = make([]Node, len(nodes))
	for i, rawNode := range nodes {
		node, problem := decodeNodeJSON(rawNode)
		if problem != nil {
			copied := *problem
			copied.Context.PageID = decoded.ID
			copied.Details = append([]diagnostic.Detail(nil), problem.Details...)
			for j := range copied.Details {
				if copied.Details[j].Label == "Поле JSON" {
					copied.Details[j].Value = "/nodes/" + strconv.Itoa(i) + copied.Details[j].Value
				}
			}
			return Page{}, &copied
		}
		decoded.Nodes[i] = node
	}
	return decoded, nil
}

// decodeNodeJSON восстанавливает точные известные поля проверенной ноды;
// flags передаёт decodeFlagsJSON, data — Data.UnmarshalJSON, потомков — себе.
func decodeNodeJSON(raw json.RawMessage) (Node, *diagnostic.Error) {
	var decoded Node
	context := diagnostic.Context{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Node{}, contractError(CodeJSON, "Не удалось прочитать ноду", "", context, err)
	}
	for _, field := range []struct {
		name   string
		target any
	}{{"id", &decoded.ID}, {"type", &decoded.Type}} {
		if err := json.Unmarshal(fields[field.name], field.target); err != nil {
			return Node{}, contractError(CodeJSON, "Не удалось прочитать поле ноды", "/"+field.name, context, err)
		}
	}
	flags, problem := decodeFlagsJSON(fields["flags"])
	if problem != nil {
		copied := *problem
		copied.Details = append([]diagnostic.Detail(nil), problem.Details...)
		for i := range copied.Details {
			if copied.Details[i].Label == "Поле JSON" {
				copied.Details[i].Value = "/flags" + copied.Details[i].Value
			}
		}
		return Node{}, &copied
	}
	decoded.Flags = flags
	for _, name := range []string{"data", "nodes"} {
		var children []json.RawMessage
		if err := json.Unmarshal(fields[name], &children); err != nil {
			return Node{}, contractError(CodeJSON, "Не удалось прочитать массив ноды", "/"+name, context, err)
		}
		if name == "data" {
			decoded.Data = make([]Data, len(children))
		} else {
			decoded.Nodes = make([]Node, len(children))
		}
		for i, child := range children {
			var problem *diagnostic.Error
			if name == "data" {
				if err := json.Unmarshal(child, &decoded.Data[i]); err != nil {
					if !errors.As(err, &problem) {
						problem = contractError(CodeJSON, "Не удалось прочитать данные ноды", "", context, err)
					}
				}
			} else {
				decoded.Nodes[i], problem = decodeNodeJSON(child)
			}
			if problem != nil {
				copied := *problem
				copied.Details = append([]diagnostic.Detail(nil), problem.Details...)
				for j := range copied.Details {
					if copied.Details[j].Label == "Поле JSON" {
						copied.Details[j].Value = "/" + name + "/" + strconv.Itoa(i) + copied.Details[j].Value
					}
				}
				return Node{}, &copied
			}
		}
	}
	return decoded, nil
}

// decodeFlagsJSON восстанавливает указатели только для присутствующих
// разрешённых флагов, сохраняя различие nil и указателя на false.
func decodeFlagsJSON(raw json.RawMessage) (NodeFlags, *diagnostic.Error) {
	var decoded NodeFlags
	context := diagnostic.Context{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return NodeFlags{}, contractError(CodeJSON, "Не удалось прочитать флаги ноды", "", context, err)
	}
	for _, field := range []struct {
		name   string
		target **bool
	}{{"speaking", &decoded.Speaking}, {"stepped", &decoded.Stepped}, {"checkable", &decoded.Checkable}} {
		if rawFlag, present := fields[field.name]; present {
			if err := json.Unmarshal(rawFlag, field.target); err != nil {
				return NodeFlags{}, contractError(CodeJSON, "Не удалось прочитать флаг ноды", "/"+field.name, context, err)
			}
		}
	}
	return decoded, nil
}
