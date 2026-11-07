package parsed

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// decodeResult принимает ровно один JSON-объект с хвостовым whitespace,
// проверяет версию и делегирует чтение обязательных полей профильным декодерам.
// Неизвестные поля игнорируются; числа не преобразуются в float64.
func decodeResult(data []byte, context diagnostic.Context) (model.Result, *diagnostic.Error) {
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return model.Result{}, inputError(CodeJSON, "Ожидался один корректный JSON-документ без комментариев.", "", context, err)
	}
	fields, problem := decodeObject(raw, []string{"formatVersion", "document", "processing", "lines", "diagnostics"}, "", context)
	if problem != nil {
		return model.Result{}, problem
	}
	var result model.Result
	if problem = decodeScalar(fields["formatVersion"], &result.Format, false, "formatVersion", context); problem != nil {
		return model.Result{}, problem
	}
	if result.Format != model.FormatVersion1 {
		return model.Result{}, inputError(CodeVersion, "Версия входного контракта не поддерживается; требуется 1.0.", "formatVersion", context, nil)
	}
	result.Document, problem = decodeDocument(fields["document"], "document", context)
	if problem != nil {
		return model.Result{}, problem
	}
	if result.Document.Metadata.DocumentID != nil {
		context.LessonID = *result.Document.Metadata.DocumentID
	}
	for _, field := range []string{"processing", "lines", "diagnostics"} {
		items, problem := decodeArray(fields[field], false, field, context)
		if problem != nil {
			return model.Result{}, problem
		}
		switch field {
		case "processing":
			result.Processing = make([]model.Processing, len(items))
		case "lines":
			result.Lines = make([]model.Line, len(items))
		case "diagnostics":
			result.Diagnostics = make([]model.Diagnostic, len(items))
		}
		for i, item := range items {
			itemField := fmt.Sprintf("%s[%d]", field, i)
			switch field {
			case "processing":
				result.Processing[i], problem = decodeProcessing(item, itemField, context)
			case "lines":
				result.Lines[i], problem = decodeLine(item, itemField, context)
			case "diagnostics":
				result.Diagnostics[i], problem = decodeDiagnostic(item, itemField, context)
			}
			if problem != nil {
				return model.Result{}, problem
			}
		}
	}
	return result, nil
}

// decodeObject читает ненулевой объект с обязательными точными ключами.
// Неизвестные ключи допустимы, а повторные получают последнее JSON-значение.
func decodeObject(raw json.RawMessage, required []string, field string, context diagnostic.Context) (map[string]json.RawMessage, *diagnostic.Error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, inputError(CodeContract, "Отсутствует обязательный объект.", field, context, nil)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		code := CodeContract
		if _, ok := err.(*json.SyntaxError); ok {
			code = CodeJSON
		}
		return nil, inputError(code, "Ожидался корректный JSON-объект.", field, context, err)
	}
	if fields == nil {
		return nil, inputError(CodeContract, "Обязательный объект не может быть null.", field, context, nil)
	}
	for _, name := range required {
		if _, ok := fields[name]; !ok {
			path := name
			if field != "" {
				path = field + "." + name
			}
			return nil, inputError(CodeContract, "Отсутствует обязательное поле.", path, context, nil)
		}
	}
	return fields, nil
}

// decodeArray читает массив в исходном порядке; null разрешён только nullable.
// Отсутствие массива всегда ошибочно, проверка элементов остаётся вызывающему.
func decodeArray(raw json.RawMessage, nullable bool, field string, context diagnostic.Context) ([]json.RawMessage, *diagnostic.Error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, inputError(CodeContract, "Отсутствует обязательный массив.", field, context, nil)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		code := CodeContract
		if _, ok := err.(*json.SyntaxError); ok {
			code = CodeJSON
		}
		return nil, inputError(code, "Ожидался корректный JSON-массив.", field, context, err)
	}
	if items == nil && !nullable {
		return nil, inputError(CodeContract, "Обязательный массив не может быть null.", field, context, nil)
	}
	return items, nil
}

// decodeScalar читает строку, boolean или целое в указатель соответствующего
// типа model. Проверяет наличие, тип и допустимость null до присваивания.
func decodeScalar(raw json.RawMessage, dst any, nullable bool, field string, context diagnostic.Context) *diagnostic.Error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return inputError(CodeContract, "Отсутствует обязательное поле.", field, context, nil)
	}
	if bytes.Equal(raw, []byte("null")) && !nullable {
		return inputError(CodeContract, "Поле не может быть null.", field, context, nil)
	}
	invalidTarget := func() *diagnostic.Error {
		return inputError(CodeContract, "Получатель поля не соответствует его типу или допустимости null.", field, context, nil)
	}
	decode := func(target any) *diagnostic.Error {
		if err := json.Unmarshal(raw, target); err != nil {
			code := CodeContract
			if _, ok := err.(*json.SyntaxError); ok {
				code = CodeJSON
			}
			return inputError(code, "Поле имеет неверный JSON-тип или недопустимую запись значения.", field, context, err)
		}
		return nil
	}
	switch target := dst.(type) {
	case *string, **string, *model.FormatVersion, *model.LineType, *model.LineEnding, *model.ElementType, *model.Code, *model.Severity, *model.Scope:
		var value *string
		if problem := decode(&value); problem != nil {
			return problem
		}
		if pointer, ok := target.(**string); ok {
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = value
			return nil
		}
		if value == nil {
			return invalidTarget()
		}
		switch pointer := target.(type) {
		case *string:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = *value
		case *model.FormatVersion:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = model.FormatVersion(*value)
		case *model.LineType:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = model.LineType(*value)
		case *model.LineEnding:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = model.LineEnding(*value)
		case *model.ElementType:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = model.ElementType(*value)
		case *model.Code:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = model.Code(*value)
		case *model.Severity:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = model.Severity(*value)
		case *model.Scope:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = model.Scope(*value)
		}
	case *bool, **bool:
		var value *bool
		if problem := decode(&value); problem != nil {
			return problem
		}
		switch pointer := target.(type) {
		case *bool:
			if pointer == nil || value == nil {
				return invalidTarget()
			}
			*pointer = *value
		case **bool:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = value
		}
	case *int, **int:
		var value *int
		if problem := decode(&value); problem != nil {
			return problem
		}
		switch pointer := target.(type) {
		case *int:
			if pointer == nil || value == nil {
				return invalidTarget()
			}
			*pointer = *value
		case **int:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = value
		}
	case *int64, **int64:
		var value *int64
		if problem := decode(&value); problem != nil {
			return problem
		}
		switch pointer := target.(type) {
		case *int64:
			if pointer == nil || value == nil {
				return invalidTarget()
			}
			*pointer = *value
		case **int64:
			if pointer == nil {
				return invalidTarget()
			}
			*pointer = value
		}
	default:
		return invalidTarget()
	}
	return nil
}

// decodeStrings читает массив строк без null-элементов, сохраняя порядок.
// Nullable относится ко всему массиву; null отличается от пустого массива.
func decodeStrings(raw json.RawMessage, nullable bool, field string, context diagnostic.Context) ([]string, *diagnostic.Error) {
	items, problem := decodeArray(raw, nullable, field, context)
	if problem != nil || items == nil {
		return nil, problem
	}
	strings := make([]string, len(items))
	for i, item := range items {
		if problem := decodeScalar(item, &strings[i], false, fmt.Sprintf("%s[%d]", field, i), context); problem != nil {
			return nil, problem
		}
	}
	return strings, nil
}
