package preview

import (
	"fmt"
	"strings"

	"github.com/verdoga/dsl-builder/catalog"
)

// OrderConflicts показывает все готовые конфликты из catalog.Conflicts
// в переданном порядке, включая каждого участника каждого конфликта.
// Выводит секцию или общую группу, канонический общий числовой вес,
// техническое имя, название, исходный order и известный Context.Path
// каждого участника. Для архивного источника сохраняет Context.FileName.
// Не подставляет document.filePath и не сокращает список до двух файлов.
// Непустой список сопровождает текстом «ОШИБКА СБОРКИ» и запретом записи
// до исправления весов; это представление готового блокирующего результата.
// Пустой список означает лишь отсутствие конфликтов весов, не готовность
// всего модуля. Схему и остальные ошибки представляет console отдельно.
func OrderConflicts(values []catalog.Conflict) string {
	if len(values) == 0 {
		return "Конфликты весов отсутствуют.\n"
	}

	var output strings.Builder
	output.WriteString("ОШИБКА СБОРКИ: одинаковые веса уроков.\n\n")
	for _, value := range values {
		output.WriteString(orderConflict(value))
		output.WriteByte('\n')
	}
	output.WriteString("Запись модуля запрещена до исправления весов.\n")
	return output.String()
}

// orderConflict формирует один блок общего веса и всех его участников;
// ведущие нули исходных весов сохраняет, строковые значения заключает в кавычки.
func orderConflict(value catalog.Conflict) string {
	var output strings.Builder
	if value.Section == nil {
		output.WriteString("Общая группа (без секций)\n")
	} else {
		fmt.Fprintf(&output, "Секция: %s\n", quotedText(*value.Section))
	}
	fmt.Fprintf(&output, "Общий числовой вес: %s\n", quotedText(value.Weight))
	for index, item := range value.Items {
		rawOrder := "не задан"
		if item.Entry.Order != nil {
			rawOrder = quotedText(*item.Entry.Order)
		}
		fmt.Fprintf(&output, "%d. %s | %s | order: %s\n",
			index+1, quotedText(item.Entry.FileName), quotedText(item.Entry.Title), rawOrder)
		if item.Context.Path != "" {
			fmt.Fprintf(&output, "   Источник: %s\n", quotedText(item.Context.Path))
		}
		if item.Context.FileName != "" {
			fmt.Fprintf(&output, "   Файл в архиве: %s\n", quotedText(item.Context.FileName))
		}
	}
	return output.String()
}
