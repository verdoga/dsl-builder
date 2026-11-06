package lesson

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// ValidateJSON проверяет один уже прочитанный JSON урока без файлового ввода.
// Координирует стандартное декодирование через Lesson.UnmarshalJSON и Validate.
// До декодирования проверяет обязательность, JSON-типы и допустимость null,
// чтобы пропущенные title, hint, flags, page.title и прочие поля нельзя было
// подменить нулевыми значениями Go. Проверяет все уровни рекурсивной модели.
// Ошибки синтаксиса и формы возвращаются до проверки зависимых значений.
// Незнакомые свойства игнорируются там, где схема их допускает;
// исключение — NodeFlags с additionalProperties=false.
// После успеха moduleio может декодировать raw через encoding/json в Lesson;
// Data.UnmarshalJSON сохраняет типизированное рекурсивное Value.
// Чтение ZIP, UTF-8 файлов и запись JSON остаются в moduleio.
func ValidateJSON(raw json.RawMessage, context diagnostic.Context) []*diagnostic.Error {
	if problems := checkLessonJSON(raw, context); len(problems) != 0 {
		return problems
	}
	var value Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		var problem *diagnostic.Error
		if errors.As(err, &problem) {
			copied := *problem
			copied.Context = context
			if problem.Context.PageID != "" {
				copied.Context.PageID = problem.Context.PageID
			}
			if problem.Context.LessonID != "" {
				copied.Context.LessonID = problem.Context.LessonID
			}
			return []*diagnostic.Error{&copied}
		}
		return []*diagnostic.Error{contractError(CodeJSON, "Не удалось декодировать урок", "", context, err)}
	}
	return Validate(&value, context)
}

// checkLessonJSON требует корневой объект и обязательные ненулевые
// formatVersion:string и document:object; документ передаёт checkDocumentJSON.
func checkLessonJSON(raw json.RawMessage, context diagnostic.Context) []*diagnostic.Error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return []*diagnostic.Error{contractError(CodeJSON, "Ожидается объект урока", "", context, err)}
	}
	var problems []*diagnostic.Error
	for _, field := range []struct{ name, kind string }{{"formatVersion", "string"}, {"document", "object"}} {
		value, problem := requireJSONField(fields, field.name, field.kind, false, "", context)
		if problem != nil {
			problems = append(problems, problem)
		} else if field.name == "document" {
			problems = append(problems, checkDocumentJSON(value, "/document", context)...)
		}
	}
	return problems
}

// checkDocumentJSON проверяет обязательные id, title, createdAt:string,
// version, lineCount:number, pages:array; subtitle необязателен, string или null.
// Каждый элемент pages обязан быть объектом по checkPageJSON.
// Числовые ограничения, дата и ID проверяются в типизированной модели.
func checkDocumentJSON(raw json.RawMessage, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return []*diagnostic.Error{contractError(CodeJSON, "Ожидается объект документа", pointer, context, err)}
	}
	if id := bytes.TrimSpace(fields["id"]); len(id) > 0 && id[0] == '"' {
		_ = json.Unmarshal(id, &context.LessonID)
	}
	var problems []*diagnostic.Error
	for _, field := range []struct{ name, kind string }{
		{"id", "string"}, {"version", "number"}, {"title", "string"}, {"subtitle", "string"},
		{"lineCount", "number"}, {"createdAt", "string"}, {"pages", "array"},
	} {
		if _, present := fields[field.name]; field.name == "subtitle" && !present {
			continue
		}
		value, problem := requireJSONField(fields, field.name, field.kind, field.name == "subtitle", pointer, context)
		if problem != nil {
			problems = append(problems, problem)
		} else if field.name == "pages" {
			var pages []json.RawMessage
			_ = json.Unmarshal(value, &pages) // Синтаксис и тип массива уже проверены.
			for i, page := range pages {
				problems = append(problems, checkPageJSON(page, pointer+"/pages/"+strconv.Itoa(i), context)...)
			}
		}
	}
	return problems
}

// checkPageJSON требует id:string, title:string или null и nodes:array;
// наличие title обязательно даже при null. Ноды передаёт checkNodeJSON.
func checkPageJSON(raw json.RawMessage, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return []*diagnostic.Error{contractError(CodeJSON, "Ожидается объект страницы", pointer, context, err)}
	}
	if id := bytes.TrimSpace(fields["id"]); len(id) > 0 && id[0] == '"' {
		_ = json.Unmarshal(id, &context.PageID)
	}
	var problems []*diagnostic.Error
	for _, field := range []struct{ name, kind string }{{"id", "string"}, {"title", "string"}, {"nodes", "array"}} {
		value, problem := requireJSONField(fields, field.name, field.kind, field.name == "title", pointer, context)
		if problem != nil {
			problems = append(problems, problem)
		} else if field.name == "nodes" {
			var nodes []json.RawMessage
			_ = json.Unmarshal(value, &nodes)
			for i, node := range nodes {
				problems = append(problems, checkNodeJSON(node, pointer+"/nodes/"+strconv.Itoa(i), context)...)
			}
		}
	}
	return problems
}

// checkNodeJSON требует id, type:string, flags:object, data, nodes:array;
// делегирует checkFlagsJSON, checkDataJSON и рекурсивную проверку дочерних нод.
func checkNodeJSON(raw json.RawMessage, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return []*diagnostic.Error{contractError(CodeJSON, "Ожидается объект ноды", pointer, context, err)}
	}
	var problems []*diagnostic.Error
	for _, field := range []struct{ name, kind string }{
		{"id", "string"}, {"type", "string"}, {"flags", "object"}, {"data", "array"}, {"nodes", "array"},
	} {
		value, problem := requireJSONField(fields, field.name, field.kind, false, pointer, context)
		if problem != nil {
			problems = append(problems, problem)
			continue
		}
		childPointer := pointer + "/" + field.name
		if field.name == "flags" {
			problems = append(problems, checkFlagsJSON(value, childPointer, context)...)
		} else if field.kind == "array" {
			var children []json.RawMessage
			_ = json.Unmarshal(value, &children)
			for i, child := range children {
				if field.name == "data" {
					problems = append(problems, checkDataJSON(child, childPointer+"/"+strconv.Itoa(i), context)...)
				} else {
					problems = append(problems, checkNodeJSON(child, childPointer+"/"+strconv.Itoa(i), context)...)
				}
			}
		}
	}
	return problems
}

// checkFlagsJSON допускает только speaking, stepped, checkable, каждое
// присутствующее значение строго boolean; null и неизвестные ключи запрещены.
// Применимость известного флага типу ноды проверяет validateFlags.
func checkFlagsJSON(raw json.RawMessage, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return []*diagnostic.Error{contractError(CodeJSON, "Ожидается объект флагов", pointer, context, err)}
	}
	var problems []*diagnostic.Error
	for _, name := range []string{"speaking", "stepped", "checkable"} {
		if _, present := fields[name]; present {
			if _, problem := requireJSONField(fields, name, "boolean", false, pointer, context); problem != nil {
				problems = append(problems, problem)
			}
		}
	}
	var unknown []string
	for name := range fields {
		if name != "speaking" && name != "stepped" && name != "checkable" {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		escaped := strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
		problems = append(problems, contractError(CodeContract, "Неизвестный флаг ноды", pointer+"/"+escaped, context, nil))
	}
	return problems
}

// checkDataJSON требует type:string, value, answers:array строк и hint:string;
// role при наличии строго строка. null в обязательных полях и answers запрещён.
// Форма value определяется type: string, EmbeddedFile-объект или массив Data;
// null запрещён. Вложенные данные передаются этому же правилу.
// Роли, количество элементов и размещение проверяет типизированная модель.
func checkDataJSON(raw json.RawMessage, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return []*diagnostic.Error{contractError(CodeJSON, "Ожидается объект данных", pointer, context, err)}
	}
	var problems []*diagnostic.Error
	var kind DataType
	for _, name := range []string{"type", "role", "value", "answers", "hint"} {
		if _, present := fields[name]; name == "role" && !present {
			continue
		}
		jsonKind := "string"
		if name == "answers" {
			jsonKind = "array"
		} else if name == "value" {
			switch kind {
			case DataText:
			case DataMedia:
				jsonKind = "object"
			case DataCollection:
				jsonKind = "array"
			default:
				if value, present := fields[name]; !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					problems = append(problems, contractError(CodeJSON, "Обязательное значение данных отсутствует или равно null", pointer+"/value", context, nil))
				}
				continue // Неизвестный дискриминатор отклонит декодирование после проверки формы.
			}
		}
		value, problem := requireJSONField(fields, name, jsonKind, false, pointer, context)
		if problem != nil {
			problems = append(problems, problem)
			continue
		}
		if name == "type" {
			_ = json.Unmarshal(value, &kind)
		} else if name == "value" && kind == DataMedia {
			problems = append(problems, checkEmbeddedJSON(value, pointer+"/value", context)...)
		} else if jsonKind == "array" {
			var elements []json.RawMessage
			_ = json.Unmarshal(value, &elements)
			for i, element := range elements {
				index := strconv.Itoa(i)
				if name == "value" {
					problems = append(problems, checkDataJSON(element, pointer+"/value/"+index, context)...)
				} else if _, problem := requireJSONField(map[string]json.RawMessage{index: element}, index, "string", false, pointer+"/answers", context); problem != nil {
					problems = append(problems, problem)
				}
			}
		}
	}
	return problems
}

// checkEmbeddedJSON требует объект с fileName, mimeType, content:string;
// все три поля обязательны и не допускают null.
func checkEmbeddedJSON(raw json.RawMessage, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return []*diagnostic.Error{contractError(CodeJSON, "Ожидается объект встроенного файла", pointer, context, err)}
	}
	var problems []*diagnostic.Error
	for _, name := range []string{"fileName", "mimeType", "content"} {
		if _, problem := requireJSONField(fields, name, "string", false, pointer, context); problem != nil {
			problems = append(problems, problem)
		}
	}
	return problems
}

// requireJSONField проверяет наличие поля и его JSON-тип kind в прочитанном
// объекте. kind — string, number, boolean, object или array; nullable разрешает
// null, но не отсутствие. Возвращает исходное поле либо nil и ошибку.
// Для необязательного поля вызывается только при его наличии.
func requireJSONField(fields map[string]json.RawMessage, name string, kind string, nullable bool, pointer string, context diagnostic.Context) (json.RawMessage, *diagnostic.Error) {
	pointer += "/" + strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
	raw, present := fields[name]
	if !present {
		return nil, contractError(CodeJSON, "Отсутствует обязательное поле", pointer, context, nil)
	}
	var checked json.RawMessage
	if err := json.Unmarshal(raw, &checked); err != nil {
		return nil, contractError(CodeJSON, "Некорректный JSON поля", pointer, context, err)
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		if nullable {
			return raw, nil
		}
		return nil, contractError(CodeJSON, "Поле не допускает null", pointer, context, nil)
	}
	actual := "number"
	switch trimmed[0] {
	case '"':
		actual = "string"
	case '{':
		actual = "object"
	case '[':
		actual = "array"
	case 't', 'f':
		actual = "boolean"
	}
	if actual != kind {
		return nil, contractError(CodeJSON, "Неверный JSON-тип поля: ожидается "+kind, pointer, context, nil)
	}
	return raw, nil
}

// decodeDataValue восстанавливает только значение union: строку, EmbeddedFile
// либо []Data. Использует encoding/json и не выполняет преобразование DSL.
// Неизвестный type или несоответствующий JSON-тип возвращают ошибку контракта.
// Имена свойств EmbeddedFile сопоставляются точно; посторонние свойства
// не могут затенить известные через нечувствительность encoding/json к регистру.
func decodeDataValue(kind DataType, raw json.RawMessage) (any, *diagnostic.Error) {
	context := diagnostic.Context{}
	if kind != DataText && kind != DataMedia && kind != DataCollection {
		return nil, contractError(CodeContract, "Неизвестный тип данных", "", context, nil)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, contractError(CodeContract, "Значение данных не допускает null", "", context, nil)
	}
	switch kind {
	case DataText:
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, contractError(CodeContract, "Ожидается строковое значение данных", "", context, err)
		}
		return value, nil
	case DataMedia:
		if problems := checkEmbeddedJSON(raw, "", context); len(problems) != 0 {
			problem := *problems[0]
			problem.Code = CodeContract
			return nil, &problem
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		var value EmbeddedFile
		_ = json.Unmarshal(fields["fileName"], &value.FileName)
		_ = json.Unmarshal(fields["mimeType"], &value.MIMEType)
		_ = json.Unmarshal(fields["content"], &value.Content)
		return value, nil
	case DataCollection:
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return nil, contractError(CodeContract, "Ожидается массив данных", "", context, err)
		}
		value := make([]Data, len(elements))
		for i, element := range elements {
			if err := json.Unmarshal(element, &value[i]); err != nil {
				var problem *diagnostic.Error
				if errors.As(err, &problem) {
					copied := *problem
					copied.Code = CodeContract
					copied.Details = append([]diagnostic.Detail(nil), problem.Details...)
					for j := range copied.Details {
						if copied.Details[j].Label == "Поле JSON" {
							copied.Details[j].Value = "/" + strconv.Itoa(i) + copied.Details[j].Value
						}
					}
					return nil, &copied
				}
				return nil, contractError(CodeContract, "Не удалось декодировать элемент коллекции", "/"+strconv.Itoa(i), context, err)
			}
		}
		return value, nil
	}
	return nil, contractError(CodeContract, "Неизвестный тип данных", "", context, nil)
}
