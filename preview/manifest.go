package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/catalog"
)

// ManifestChanges представляет готовый результат catalog.Changes в исходном
// порядке: селектор файла/секции, поле и точные «Было»/«Станет».
// Подходит и для предложения перед решением, и для принятого плана;
// сам не объявляет изменения подтверждёнными. Весь список показывается,
// включая изменения курса, юнита, секций, порядка уроков и всех полей Entry.
// Пустой список означает отсутствие изменений именно логического манифеста.
// Не добавляет корневой createdAt: его исключает catalog.Changes;
// сообщение о его обновлении при записи формирует владелец операции.
func ManifestChanges(changes []catalog.Change) (string, error) {
	if len(changes) == 0 {
		return "Изменений логического манифеста нет.\n", nil
	}
	var report strings.Builder
	report.WriteString("Изменения манифеста:\n")
	for _, change := range changes {
		text, err := manifestChange(change)
		if err != nil {
			return "", err
		}
		report.WriteString(text)
	}
	return report.String(), nil
}

// manifestChange выводит одну готовую правку без повторного сравнения.
// entry показывает все предоставленные поля целой записи; массивы sections
// и lessons сохраняют точный порядок. JSON нод сюда не передаётся.
func manifestChange(change catalog.Change) (string, error) {
	switch change.Field {
	case "course.title", "course.unit", "sections":
		if change.FileName != "" || change.Section != nil {
			return "", fmt.Errorf("невозможно представить изменение манифеста: несогласованный селектор")
		}
	case "lessons":
		if change.FileName != "" {
			return "", fmt.Errorf("невозможно представить порядок уроков: несогласованный селектор")
		}
	case "section", "entry", "id", "fileName", "order", "title", "subtitle", "lineCount", "createdAt", "version":
		if change.FileName == "" || change.Section != nil {
			return "", fmt.Errorf("невозможно представить изменение записи манифеста: несогласованный селектор")
		}
	default:
		return "", fmt.Errorf("невозможно представить изменение манифеста: неизвестное поле")
	}
	before, err := manifestValue(change.Before)
	if err != nil {
		return "", err
	}
	after, err := manifestValue(change.After)
	if err != nil {
		return "", err
	}
	var report strings.Builder
	if change.FileName != "" {
		fmt.Fprintf(&report, "Файл: %s\n", quotedText(change.FileName))
	} else if change.Section != nil {
		fmt.Fprintf(&report, "Секция: %s\n", quotedText(*change.Section))
	} else if change.Field == "lessons" {
		report.WriteString("Группа: без секции\n")
	}
	fmt.Fprintf(&report, "Поле: %s\n  Было: %s\n  Станет: %s\n", change.Field, before, after)
	return report.String(), nil
}

// manifestValue отображает nil RawMessage как «отсутствует», JSON null как
// «null», строку как строку в кавычках и остальные JSON-значения компактно.
// Не использует float64: числовые токены и ведущие нули внутри строк точны.
// Некорректный JSON даёт error без копирования его содержимого в сообщение.
func manifestValue(value json.RawMessage) (string, error) {
	if value == nil {
		return "отсутствует", nil
	}
	if !utf8.Valid(value) {
		return "", fmt.Errorf("невозможно представить значение манифеста: некорректный JSON")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return "", fmt.Errorf("невозможно представить значение манифеста: некорректный JSON")
	}
	if compact.Bytes()[0] == '"' {
		var text string
		if err := json.Unmarshal(compact.Bytes(), &text); err != nil {
			return "", fmt.Errorf("невозможно представить строку манифеста: некорректный JSON")
		}
		return quotedText(text), nil
	}
	// Compact сохраняет числовые токены, порядок и все поля объекта. Сырые
	// непечатные руны внутри вложенных строк представляем JSON-экранированием.
	var safe strings.Builder
	for _, r := range compact.String() {
		if strconv.IsPrint(r) {
			safe.WriteRune(r)
		} else if r <= 0xffff {
			fmt.Fprintf(&safe, "\\u%04x", r)
		} else {
			high, low := utf16.EncodeRune(r)
			fmt.Fprintf(&safe, "\\u%04x\\u%04x", high, low)
		}
	}
	return safe.String(), nil
}
