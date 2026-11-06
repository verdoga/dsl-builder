package identity

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// NodeIDParts — разобранные части ID ноды; нужны для сохранения базового
// номера при замене и чтения суффикса без зависимости от модели урока.
type NodeIDParts struct {
	// Prefix — проверенный ID урока в нижнем регистре.
	Prefix string

	// Number — положительный базовый номер ASCII-цифрами без ведущих нулей.
	Number string

	// Revision — положительная ревизия ASCII-цифрами без ведущих нулей;
	// пустая строка означает отсутствие суффикса, а не ревизию ноль.
	Revision string
}

// ParseNodeID проверяет полную каноническую форму prefix-nNNN
// либо prefix-nNNN-vNN. Prefix соответствует ValidateLessonID
// и равен своему LessonKey; ограничений DSL для Prefix нет.
// Формы a:b-n001 и lesson+1-n001 допустимы.
// Номер положителен, ширина минимум 3; суффикс при наличии положителен,
// ширина минимум 2; лишние нули
// сверх минимальной ширины запрещены. n001-v06 и n1000-v100 допустимы.
// Терминальные -n с номером и необязательным -v с ревизией выделяются
// справа; такие же фрагменты внутри Prefix не создают неоднозначности.
// Ревизия суффикса не сверяется с document.version и историей изменений:
// это обязанность вызывающего пакета, синтаксически v01 допустим.
// Ошибка имеет CodeInvalidNodeID; разобранные числа не содержат padding.
func ParseNodeID(value string) (NodeIDParts, *diagnostic.Error) {
	invalid := &diagnostic.Error{
		Code:   CodeInvalidNodeID,
		Reason: "ID ноды должен содержать префикс в нижнем регистре, канонический положительный номер и необязательную положительную ревизию.",
		Details: []diagnostic.Detail{
			{Label: "ID ноды", Value: value},
		},
	}
	nodeAt := strings.LastIndex(value, "-n")
	if nodeAt <= 0 {
		return NodeIDParts{}, invalid
	}
	prefix := value[:nodeAt]
	if ValidateLessonID(prefix) != nil || prefix != LessonKey(prefix) {
		return NodeIDParts{}, invalid
	}
	ending := value[nodeAt+2:]
	numbers := []string{ending}
	if revisionAt := strings.LastIndex(ending, "-v"); revisionAt >= 0 {
		numbers = []string{ending[:revisionAt], ending[revisionAt+2:]}
	}
	for i, digits := range numbers {
		width := 3
		if i == 1 {
			width = 2
		}
		if len(digits) < width {
			return NodeIDParts{}, invalid
		}
		for j := 0; j < len(digits); j++ {
			if digits[j] < '0' || digits[j] > '9' {
				return NodeIDParts{}, invalid
			}
		}
		unpadded := strings.TrimLeft(digits, "0")
		if unpadded == "" || len(digits) > width && digits[0] == '0' {
			return NodeIDParts{}, invalid
		}
		numbers[i] = unpadded
	}
	parts := NodeIDParts{Prefix: prefix, Number: numbers[0]}
	if len(numbers) == 2 {
		parts.Revision = numbers[1]
	}
	return parts, nil
}

// FormatNodeID проверяет lessonID через ValidateLessonID,
// положительный number и revision.
// Возвращает LessonKey(lessonID) с -n и номером минимальной ширины 3.
// Например, A:B, 1 и пустая revision дают a:b-n001.
// Пустая revision означает отсутствие суффикса; иначе добавляется -v
// с положительной ревизией минимальной ширины 2, без ограничения длины.
// Для замены суффикса вызывающий код передаёт Prefix и Number из ParseNodeID
// и выбранную им новую глобальную ревизию; прежний суффикс не переносится.
// Решение об отсутствии суффикса в первой ревизии принимает вызывающий код.
// Ошибки: CodeInvalidLessonID либо CodeInvalidNumber; Details указывает,
// какое значение неверно: базовый номер или ревизия.
func FormatNodeID(lessonID string, number string, revision string) (string, *diagnostic.Error) {
	if err := ValidateLessonID(lessonID); err != nil {
		return "", err
	}
	numbers := [2]string{number, revision}
	for i, digits := range numbers {
		if i == 1 && digits == "" {
			continue
		}
		valid := true
		for j := 0; j < len(digits); j++ {
			if digits[j] < '0' || digits[j] > '9' {
				valid = false
				break
			}
		}
		unpadded := strings.TrimLeft(digits, "0")
		if !valid || unpadded == "" {
			label := "Базовый номер ноды"
			if i == 1 {
				label = "Ревизия ноды"
			}
			return "", &diagnostic.Error{
				Code:   CodeInvalidNumber,
				Reason: "Ожидается непустая последовательность ASCII-цифр с положительным значением.",
				Details: []diagnostic.Detail{
					{Label: label, Value: digits},
				},
			}
		}
		width := 3
		if i == 1 {
			width = 2
		}
		if len(unpadded) < width {
			unpadded = strings.Repeat("0", width-len(unpadded)) + unpadded
		}
		numbers[i] = unpadded
	}
	result := LessonKey(lessonID) + "-n" + numbers[0]
	if revision != "" {
		result += "-v" + numbers[1]
	}
	return result, nil
}
