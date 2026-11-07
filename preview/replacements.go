package preview

import (
	"errors"
	"fmt"
	"strings"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/module"
)

// nodeReplacement показывает страницу и одну пару полных ID «было → станет»
// по pageIndex и change.Path, только при сохранённой структуре плана.
// Перечисляет русские названия изменённых собственных полей из change.Fields,
// но не их значения. Не приписывает изменения всем последующим нодам.
func nodeReplacement(before *module.File, after *module.File, pageIndex int, change compare.NodeChange) (string, error) {
	oldID, err := nodeID(before, pageIndex, change.Path)
	if err != nil {
		return "", err
	}
	newID, err := nodeID(after, pageIndex, change.Path)
	if err != nil {
		return "", err
	}
	oldLesson, newLesson := before.Lesson(), after.Lesson()
	oldPage, newPage := oldLesson.Document.Pages[pageIndex], newLesson.Document.Pages[pageIndex]
	if oldLesson.Document.ID != newLesson.Document.ID || before.FileName() != after.FileName() || oldPage.ID != newPage.ID {
		return "", errors.New("Снимки не согласованы для показа замены отдельной ноды")
	}
	if len(change.Fields) == 0 {
		return "", errors.New("Для показа замены ноды нужны изменённые собственные поля")
	}
	fields := make([]string, 0, len(change.Fields))
	seen := make(map[compare.NodeField]bool)
	for _, field := range change.Fields {
		if seen[field] {
			return "", errors.New("Селектор изменённых полей ноды не согласован")
		}
		seen[field] = true
		switch field {
		case compare.NodeTypeChanged:
			fields = append(fields, "тип")
		case compare.NodeFlagsChanged:
			fields = append(fields, "флаги")
		case compare.NodeDataChanged:
			fields = append(fields, "данные")
		default:
			return "", errors.New("Селектор изменённых полей ноды неизвестен")
		}
	}
	return fmt.Sprintf("Страница %s, название: %s.\nБудет заменена нода: %s → %s.\nИзменённые собственные поля: %s.\n",
		quotedText(oldPage.ID), optionalText(newPage.Title), quotedText(oldID), quotedText(newID), strings.Join(fields, ", ")), nil
}

// pageReplacement показывает ID и название страницы, все структурные причины
// и сохранение ID страницы. В отдельных списках «Будут удалены» и
// «Будут созданы» перечисляет все старые/новые ноды, включая вложенные.
// Списки независимы, могут быть пустыми и иметь разную длину; пары не строятся.
func pageReplacement(before *module.File, after *module.File, change compare.PageChange) (string, error) {
	if !change.ReplaceNodes || len(change.Nodes) != 0 {
		return "", errors.New("Селектор полной замены нод страницы не согласован")
	}
	oldIDs, err := nodeIDs(before, change.Index)
	if err != nil {
		return "", err
	}
	newIDs, err := nodeIDs(after, change.Index)
	if err != nil {
		return "", err
	}
	oldLesson, newLesson := before.Lesson(), after.Lesson()
	oldPage, newPage := oldLesson.Document.Pages[change.Index], newLesson.Document.Pages[change.Index]
	if oldLesson.Document.ID != newLesson.Document.ID || before.FileName() != after.FileName() || oldPage.ID != newPage.ID {
		return "", errors.New("Снимки не согласованы для показа полной замены нод страницы")
	}
	return fmt.Sprintf("Будут заменены все ноды страницы %s, название: %s.\n%sID страницы будет сохранён.\nБудут удалены: %s\nБудут созданы: %s\n",
		quotedText(oldPage.ID), optionalText(newPage.Title), replacementReasons(change.Reasons), idList(oldIDs), idList(newIDs)), nil
}

// lessonReplacement явно объявляет полную замену урока и все её причины.
// Указывает сохранение ID урока и технического имени. Через pageIDs отдельно
// выводит старые и новые страницы, затем через nodeIDs — старые и новые ноды
// каждой стороны, сгруппированные по её собственным страницам.
// Не сопоставляет страницы по позиции, первичному ID или названию;
// не сворачивает фактические ID в диапазоны и не вычисляет новые номера.
func lessonReplacement(before *module.File, after *module.File, reasons []compare.Reason) (string, error) {
	oldPages, err := pageIDs(before)
	if err != nil {
		return "", err
	}
	newPages, err := pageIDs(after)
	if err != nil {
		return "", err
	}
	oldLesson, newLesson := before.Lesson(), after.Lesson()
	if oldLesson.Document.ID != newLesson.Document.ID || before.FileName() != after.FileName() {
		return "", errors.New("Снимки не согласованы для показа полной замены урока")
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Будет выполнена полная замена урока.\nID урока будет сохранён: %s.\nТехническое имя будет сохранено: %s.\n",
		quotedText(oldLesson.Document.ID), quotedText(before.FileName()))
	text.WriteString(replacementReasons(reasons))
	fmt.Fprintf(&text, "Старые страницы: %s\nНовые страницы: %s\nСтарые ноды по страницам:\n", idList(oldPages), idList(newPages))
	if len(oldPages) == 0 {
		text.WriteString("нет\n")
	}
	for index, pageID := range oldPages {
		ids, err := nodeIDs(before, index)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&text, "%s: %s\n", quotedText(pageID), idList(ids))
	}
	text.WriteString("Новые ноды по страницам:\n")
	if len(newPages) == 0 {
		text.WriteString("нет\n")
	}
	for index, pageID := range newPages {
		ids, err := nodeIDs(after, index)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&text, "%s: %s\n", quotedText(pageID), idList(ids))
	}
	return text.String(), nil
}

// replacementReasons выводит все Description в исходном порядке и известные
// OldPointer/NewPointer с явными подписями старого снимка и кандидата.
// Адрес кандидата не выдаёт за новый ID, не подменяет конкретную причину
// общим сообщением о больших изменениях и не добавляет собственных причин.
func replacementReasons(reasons []compare.Reason) string {
	if len(reasons) == 0 {
		return "Причины: нет\n"
	}
	var text strings.Builder
	for _, reason := range reasons {
		fmt.Fprintf(&text, "Причина: %s\n", quotedText(reason.Description))
		if reason.OldPointer != "" {
			fmt.Fprintf(&text, "Адрес в старом снимке: %s\n", quotedText(reason.OldPointer))
		}
		if reason.NewPointer != "" {
			fmt.Fprintf(&text, "Адрес в кандидате: %s\n", quotedText(reason.NewPointer))
		}
	}
	return text.String()
}
