package manifest

import (
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// ValidateShape проверяет required, additionalProperties, типы и null до
// преобразования уже декодированного JSON в Manifest. moduleio передаёт
// обычное дерево encoding/json: map[string]any, []any, string, json.Number,
// bool и nil, полученное с UseNumber. Файлы и байты функция не читает.
// Вход обязан представлять ровно одно синтаксически корректное JSON-значение;
// его декодирование, UTF-8 и отсутствие хвостовых данных проверяет moduleio.
// Числовой тип float64 не принимается: точность должна сохраняться до проверки.
// Проверяет корневой объект и его три обязательных поля, затем делегирует
// вложенные объекты; неизвестные поля запрещены на каждом уровне схемы.
// Отсутствие order/subtitle отличается от допустимого явного null;
// unit:null и null вместо массива/объекта/обязательного скаляра запрещены.
// Значения const/pattern/minimum/minItems/uniqueItems проверяет Validate;
// для полной проверки прочитанного манифеста необходимы оба вызова.
// Успех — пустой срез; ошибки упорядочены по полям схемы и индексам массивов,
// неизвестные поля — лексикографически; JSON Pointer помещается в Details.
func ValidateShape(value any, context diagnostic.Context) []*diagnostic.Error {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return []*diagnostic.Error{{Code: CodeShape, Reason: "Манифест должен быть объектом JSON.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: ""}}}}
	}
	var errors []*diagnostic.Error
	for _, field := range []string{"formatVersion", "createdAt", "course"} {
		child, exists := object[field]
		pointer := "/" + field
		reason := ""
		if !exists {
			reason = "Отсутствует обязательное поле."
		} else if field == "course" {
			errors = append(errors, validateCourseShape(child, pointer, context)...)
		} else if _, ok := child.(string); !ok {
			reason = "Поле должно быть строкой JSON."
		}
		if reason != "" {
			errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: reason, Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}}})
		}
	}
	var unknown []string
	for field := range object {
		if field != "formatVersion" && field != "createdAt" && field != "course" {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		pointer := "/" + strings.ReplaceAll(strings.ReplaceAll(field, "~", "~0"), "/", "~1")
		errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: "Дополнительное поле запрещено схемой.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}}})
	}
	return errors
}

// validateCourseShape проверяет объект и ровно один набор полей:
// title/unit либо title/lessons/sections, затем формы выбранных потомков.
// Наличие unit учитывается независимо от значения; unit:null не равно отсутствию.
func validateCourseShape(value any, pointer string, context diagnostic.Context) []*diagnostic.Error {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return []*diagnostic.Error{{Code: CodeShape, Reason: "Ожидается объект JSON.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}}}}
	}
	var errors []*diagnostic.Error
	fields := []string{"title", "unit"}
	if _, hasUnit := object["unit"]; !hasUnit {
		fields = []string{"title", "lessons", "sections"}
	}
	for _, field := range fields {
		child, exists := object[field]
		childPointer := pointer + "/" + field
		reason := ""
		if !exists {
			reason = "Отсутствует обязательное поле."
		} else {
			switch field {
			case "title":
				if _, ok := child.(string); !ok {
					reason = "Поле должно быть строкой JSON."
				}
			case "unit":
				errors = append(errors, validateUnitShape(child, childPointer, context)...)
			case "lessons":
				errors = append(errors, validateEntriesShape(child, childPointer, context)...)
			case "sections":
				errors = append(errors, validateSectionsShape(child, childPointer, context)...)
			}
		}
		if reason != "" {
			errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: reason, Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: childPointer}}})
		}
	}
	var unknown []string
	for field := range object {
		if !slices.Contains(fields, field) {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		childPointer := pointer + "/" + strings.ReplaceAll(strings.ReplaceAll(field, "~", "~0"), "/", "~1")
		errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: "Дополнительное поле запрещено схемой.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: childPointer}}})
	}
	return errors
}

// validateUnitShape проверяет обязательные title:string, lessons:array,
// sections:array без дополнительных полей, затем формы их элементов.
func validateUnitShape(value any, pointer string, context diagnostic.Context) []*diagnostic.Error {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return []*diagnostic.Error{{Code: CodeShape, Reason: "Ожидается объект JSON.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}}}}
	}
	var errors []*diagnostic.Error
	fields := []string{"title", "lessons", "sections"}
	for _, field := range fields {
		child, exists := object[field]
		childPointer := pointer + "/" + field
		reason := ""
		if !exists {
			reason = "Отсутствует обязательное поле."
		} else {
			switch field {
			case "title":
				if _, ok := child.(string); !ok {
					reason = "Поле должно быть строкой JSON."
				}
			case "lessons":
				errors = append(errors, validateEntriesShape(child, childPointer, context)...)
			case "sections":
				errors = append(errors, validateSectionsShape(child, childPointer, context)...)
			}
		}
		if reason != "" {
			errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: reason, Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: childPointer}}})
		}
	}
	var unknown []string
	for field := range object {
		if !slices.Contains(fields, field) {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		childPointer := pointer + "/" + strings.ReplaceAll(strings.ReplaceAll(field, "~", "~0"), "/", "~1")
		errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: "Дополнительное поле запрещено схемой.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: childPointer}}})
	}
	return errors
}

// validateSectionsShape проверяет тип array и передаёт каждый элемент
// validateSectionShape; null вместо массива или объекта не разрешён.
func validateSectionsShape(value any, pointer string, context diagnostic.Context) []*diagnostic.Error {
	elements, ok := value.([]any)
	if !ok || elements == nil {
		return []*diagnostic.Error{{Code: CodeShape, Reason: "Ожидается массив JSON.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}}}}
	}
	var errors []*diagnostic.Error
	for index, element := range elements {
		errors = append(errors, validateSectionShape(element, pointer+"/"+strconv.Itoa(index), context)...)
	}
	return errors
}

// validateSectionShape проверяет ровно title:string и lessons:array,
// обязательность обоих полей и формы записей через validateEntriesShape.
func validateSectionShape(value any, pointer string, context diagnostic.Context) []*diagnostic.Error {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return []*diagnostic.Error{{Code: CodeShape, Reason: "Ожидается объект JSON.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}}}}
	}
	var errors []*diagnostic.Error
	fields := []string{"title", "lessons"}
	for _, field := range fields {
		child, exists := object[field]
		childPointer := pointer + "/" + field
		reason := ""
		if !exists {
			reason = "Отсутствует обязательное поле."
		} else {
			switch field {
			case "title":
				if _, ok := child.(string); !ok {
					reason = "Поле должно быть строкой JSON."
				}
			case "lessons":
				errors = append(errors, validateEntriesShape(child, childPointer, context)...)
			}
		}
		if reason != "" {
			errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: reason, Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: childPointer}}})
		}
	}
	var unknown []string
	for field := range object {
		if !slices.Contains(fields, field) {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		childPointer := pointer + "/" + strings.ReplaceAll(strings.ReplaceAll(field, "~", "~0"), "/", "~1")
		errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: "Дополнительное поле запрещено схемой.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: childPointer}}})
	}
	return errors
}

// validateEntriesShape проверяет тип array и формы всех записей;
// ограничение длины и зависимость null order от длины остаются в Validate.
func validateEntriesShape(value any, pointer string, context diagnostic.Context) []*diagnostic.Error {
	elements, ok := value.([]any)
	if !ok || elements == nil {
		return []*diagnostic.Error{{Code: CodeShape, Reason: "Ожидается массив JSON.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}}}}
	}
	var errors []*diagnostic.Error
	for index, element := range elements {
		errors = append(errors, validateEntryShape(element, pointer+"/"+strconv.Itoa(index), context)...)
	}
	return errors
}

// validateEntryShape проверяет восемь обязательных полей без дополнительных:
// id/fileName/title/createdAt — string; order/subtitle — string либо null;
// lineCount/version — json.Number. Значения чисел проверяются после формы.
func validateEntryShape(value any, pointer string, context diagnostic.Context) []*diagnostic.Error {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return []*diagnostic.Error{{Code: CodeShape, Reason: "Запись урока должна быть объектом JSON.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}}}}
	}
	if id, ok := object["id"].(string); ok && id != "" {
		context.LessonID = id
	}
	if fileName, ok := object["fileName"].(string); ok && fileName != "" {
		context.FileName = fileName
	}
	var errors []*diagnostic.Error
	fields := []string{"id", "fileName", "order", "title", "subtitle", "lineCount", "createdAt", "version"}
	for _, field := range fields {
		child, exists := object[field]
		reason := ""
		if !exists {
			reason = "Отсутствует обязательное поле."
		} else {
			switch field {
			case "lineCount", "version":
				if _, ok := child.(json.Number); !ok {
					reason = "Поле должно быть числом JSON, декодированным с UseNumber."
				}
			case "order", "subtitle":
				if _, ok := child.(string); !ok && child != nil {
					reason = "Поле должно быть строкой JSON либо null."
				}
			default:
				if _, ok := child.(string); !ok {
					reason = "Поле должно быть строкой JSON."
				}
			}
		}
		if reason != "" {
			errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: reason, Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer + "/" + field}}})
		}
	}
	var unknown []string
	for field := range object {
		if !slices.Contains(fields, field) {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		childPointer := pointer + "/" + strings.ReplaceAll(strings.ReplaceAll(field, "~", "~0"), "/", "~1")
		errors = append(errors, &diagnostic.Error{Code: CodeShape, Reason: "Дополнительное поле запрещено схемой.", Context: context, Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: childPointer}}})
	}
	return errors
}
