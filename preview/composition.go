package preview

import (
	"fmt"
	"strings"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/module"
)

// Composition показывает курс, присутствующий юнит и весь текущий состав.
// Размещение берёт только из state, файлы — из value.Files; value.Manifest
// не читает. Допускает незавершённый catalog до Export и module.Validate:
// отсутствующие/неверные веса, конфликты и нераспределённые записи сохраняются.
// newFileNames — явный список добавляемых файлов от владельца операции;
// при создании пуст. Только эти строки получают пометку «НОВЫЙ» и ID урока.
// Не выводит версии отдельных уроков, в том числе при новой сборке.
// Небольшой координатор compositionHeading, compositionGroups и totals.
// Предусловие: у каждой записи есть ровно один готовый File; набор имён
// записей равен набору Files. Ошибка сопоставления не скрывает отдельный
// блок OrderConflicts, который вызывающий код может показать независимо.
func Composition(state *catalog.State, value *module.Module, newFileNames []string) (string, error) {
	if state == nil || value == nil {
		return "", fmt.Errorf("Для просмотра состава нужны размещение и модуль.")
	}
	groups, err := compositionGroups(state, value.Files, newFileNames)
	if err != nil {
		return "", err
	}
	summary, err := totals(value.Files)
	if err != nil {
		return "", err
	}
	return compositionHeading(state) + "\n" + groups + "\n" + summary, nil
}

// compositionHeading выводит название курса и только присутствующий юнит.
// Ничего не извлекает из имени архива, путей или содержимого уроков.
func compositionHeading(state *catalog.State) string {
	var result strings.Builder
	result.WriteString("ПРЕДВАРИТЕЛЬНЫЙ ПРОСМОТР\n\nКурс: " + quotedText(state.CourseTitle) + "\n")
	if state.UnitTitle != nil {
		result.WriteString("Юнит: " + optionalText(state.UnitTitle) + "\n")
	}
	return result.String()
}

// compositionGroups сохраняет порядок Sections и Items, не сортируя веса.
// Секции нумеруются с 1; временный вес показывается только при его наличии.
// Пустая рабочая секция помечается как пустая, без обещания её записи в JSON.
// При Sectioned=true записи без секции идут в блок «Не распределены»;
// при false — «Без секций». Все записи показываются ровно один раз.
// Для строки вызывает compositionLesson; не выбирает первый из дублей файла.
func compositionGroups(state *catalog.State, files []*module.File, newFileNames []string) (string, error) {
	if state == nil {
		return "", fmt.Errorf("Не задано размещение уроков для просмотра состава.")
	}
	byName := make(map[string]*module.File, len(files))
	for _, file := range files {
		if file == nil {
			return "", fmt.Errorf("Отсутствует снимок файла в составе.")
		}
		name := file.FileName()
		if name == "" || byName[name] != nil {
			return "", fmt.Errorf("Техническое имя не выбирает единственный файл состава.")
		}
		byName[name] = file
	}
	if len(state.Items) != len(files) {
		return "", fmt.Errorf("Наборы записей размещения и файлов состава не совпадают.")
	}
	newNames := make(map[string]bool, len(newFileNames))
	for _, name := range newFileNames {
		if byName[name] == nil || newNames[name] {
			return "", fmt.Errorf("Список новых уроков не выбирает однозначно файлы состава.")
		}
		newNames[name] = true
	}
	sectionIndex := make(map[string]int, len(state.Sections))
	for index, section := range state.Sections {
		if _, exists := sectionIndex[section.Title]; exists {
			return "", fmt.Errorf("Имя секции не выбирает единственную группу состава.")
		}
		sectionIndex[section.Title] = index
	}
	groups := make([][]string, len(state.Sections)+1)
	seen := make(map[string]bool, len(state.Items))
	for _, item := range state.Items {
		name := item.Entry.FileName
		file := byName[name]
		if file == nil || seen[name] {
			return "", fmt.Errorf("Запись размещения не выбирает единственный файл состава.")
		}
		seen[name] = true
		group := len(state.Sections)
		if item.Section != nil {
			var exists bool
			group, exists = sectionIndex[*item.Section]
			if !exists {
				return "", fmt.Errorf("Размещение урока ссылается на недоступную секцию.")
			}
		}
		line, err := compositionLesson(item, file, newNames[name])
		if err != nil {
			return "", err
		}
		groups[group] = append(groups[group], line)
	}
	var result strings.Builder
	for index, section := range state.Sections {
		fmt.Fprintf(&result, "%d. %s", index+1, quotedText(section.Title))
		if section.Weight != nil {
			result.WriteString(" | Временный вес: " + optionalText(section.Weight))
		}
		result.WriteByte('\n')
		if len(groups[index]) == 0 {
			result.WriteString("(пустая рабочая секция)\n")
		} else {
			result.WriteString(strings.Join(groups[index], "\n") + "\n")
		}
		result.WriteByte('\n')
	}
	ungrouped := groups[len(state.Sections)]
	if len(ungrouped) != 0 || len(state.Sections) == 0 {
		if state.Sectioned {
			result.WriteString("Не распределены\n")
		} else {
			result.WriteString("Без секций\n")
		}
		if len(ungrouped) == 0 {
			result.WriteString("(пусто)\n")
		} else {
			result.WriteString(strings.Join(ungrouped, "\n") + "\n")
		}
	}
	return result.String(), nil
}

// compositionLesson выводит одной строкой техническое имя, название,
// присутствующий подзаголовок, исходный order, lineCount, число страниц
// и всех вложенных нод. Метаданные берёт из Item.Entry, счётчики — из File.
// nil Order отображает как «не задан»; неверную строку сохраняет буквально.
// isNew только отображает уже принятое отнесение к добавляемым урокам.
func compositionLesson(item catalog.Item, file *module.File, isNew bool) (string, error) {
	if file == nil || item.Entry.FileName == "" || file.FileName() != item.Entry.FileName {
		return "", fmt.Errorf("Запись урока не соответствует выбранному файлу состава.")
	}
	pages, nodes, err := lessonCounts(file)
	if err != nil {
		return "", err
	}
	entry := item.Entry
	var result strings.Builder
	result.WriteString(quotedText(entry.FileName) + " | " + quotedText(entry.Title))
	if entry.Subtitle != nil {
		result.WriteString(" | " + optionalText(entry.Subtitle))
	}
	weight := "не задан"
	if entry.Order != nil {
		weight = optionalText(entry.Order)
	}
	// Убираются только оформляющие кавычки: запись числа сохраняется,
	// а управляющие символы даже незавершённых метаданных остаются видимыми.
	lineCount := quotedText(entry.LineCount.String())
	fmt.Fprintf(&result, " | order: %s | Строк: %s | Страниц: %d | Нод: %d",
		weight, lineCount[1:len(lineCount)-1], pages, nodes)
	if isNew {
		result.WriteString(" | НОВЫЙ | ID: " + quotedText(entry.ID))
	}
	return result.String(), nil
}

// totals выводит суммарное количество уроков, страниц и всех вложенных нод
// из полного набора файлов; пустой набор даёт нули, а не ошибку схемы.
// Координирует lessonCounts; это статистика отображения, не валидация.
func totals(files []*module.File) (string, error) {
	var pages, nodes int
	for _, file := range files {
		filePages, fileNodes, err := lessonCounts(file)
		if err != nil {
			return "", err
		}
		pages += filePages
		nodes += fileNodes
	}
	return fmt.Sprintf("Итого: уроков %d; страниц %d; нод %d.\n", len(files), pages, nodes), nil
}

// lessonCounts получает количество страниц и нод любой вложенности
// из File.Lesson, не читая собственные Data и не пересчитывая lineCount.
// Счётчики соответствуют реально размещённым в памяти массивам.
func lessonCounts(file *module.File) (pages int, nodes int, err error) {
	if file == nil {
		return 0, 0, fmt.Errorf("Отсутствует файл для подсчёта страниц и нод.")
	}
	snapshot := file.Lesson()
	if snapshot == nil {
		return 0, 0, fmt.Errorf("Отсутствует снимок урока для подсчёта страниц и нод.")
	}
	pages = len(snapshot.Document.Pages)
	for _, page := range snapshot.Document.Pages {
		pending := page.Nodes
		for len(pending) != 0 {
			last := len(pending) - 1
			node := pending[last]
			pending = pending[:last]
			nodes++
			pending = append(pending, node.Nodes...)
		}
	}
	return pages, nodes, nil
}
