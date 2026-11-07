package console

import "github.com/verdoga/dsl-builder/app"

// editLessonWeight выбирает один файл, читает обязательную строку веса
// и вызывает ProposeLessonWeight(snapshot.Token, fileName, raw), затем
// reviewEdit. Подходит и для отсутствующего веса. Не использует int/float64,
// не снимает ведущие нули, не проверяет положительность вместо владельца.
func (d *dialog) editLessonWeight(snapshot app.Snapshot) error {
	fileName, accepted, err := d.chooseLesson(snapshot)
	if err != nil || !accepted {
		return err
	}
	raw, accepted, err := d.readValue("Вес урока «"+displayValue(fileName)+"»:", false)
	if err != nil || !accepted {
		return err
	}
	edit, issues := d.operation.ProposeLessonWeight(snapshot.Token, fileName, raw)
	return d.reviewEdit(snapshot, edit, issues)
}

// editGroupWeights выбирает группу через chooseGroup, собирает полный набор
// collectLessonWeights и лишь затем вызывает ProposeLessonWeights с исходным
// snapshot.Token. Никаких отдельных ProposeLessonWeight во время сбора нет.
// Ошибка итоговой проверки показывает диагностику и позволяет новый ввод
// всей группы через меню; отказ от набора не меняет принятые веса.
func (d *dialog) editGroupWeights(snapshot app.Snapshot) error {
	section, accepted, err := d.chooseGroup(snapshot)
	if err != nil || !accepted {
		return err
	}
	weights, accepted, err := d.collectLessonWeights(snapshot, section)
	if err != nil || !accepted {
		return err
	}
	edit, issues := d.operation.ProposeLessonWeights(snapshot.Token, section, weights)
	return d.reviewEdit(snapshot, edit, issues)
}

// collectLessonWeights последовательно спрашивает строковый вес каждой
// записи выбранной группы в порядке Catalog.Items. Ключи — Entry.FileName,
// без чужих файлов. readValue перед каждым значением предоставляет отдельную
// числовую отмену всей группы, включая уже набранные ответы. Возвращает карту
// только при полном вводе; при отмене/EOF/ошибке карта nil, app не вызывается.
func (d *dialog) collectLessonWeights(snapshot app.Snapshot, section *string) (weights map[string]string, accepted bool, err error) {
	if snapshot.Catalog == nil {
		return nil, false, nil
	}
	weights = make(map[string]string)
	for _, item := range snapshot.Catalog.Items {
		if (item.Section == nil) != (section == nil) || (section != nil && *item.Section != *section) {
			continue
		}
		raw, accepted, err := d.readValue("Вес урока «"+displayValue(item.Entry.FileName)+"»:", false)
		if err != nil || !accepted {
			return nil, false, err
		}
		weights[item.Entry.FileName] = raw
	}
	return weights, true, nil
}

// editSectionWeights работает с независимым Snapshot операции: локальный
// срез sections берёт из snapshot.Catalog.Sections без объявления нового типа.
// Для каждой секции читает raw через readValue, сохраняет отдельный *string
// в sections[i].Weight. После полного набора вызывает
// Operation.ProposeSectionWeights(snapshot.Token, sections) и reviewEdit.
// Отмена любого вопроса отбрасывает весь локальный набор. Прямой импорт catalog
// не нужен; проверки полноты, положительности, равных весов и сортировка — app.
func (d *dialog) editSectionWeights(snapshot app.Snapshot) error {
	if snapshot.Catalog == nil {
		return nil
	}
	sections := snapshot.Catalog.Sections
	sections = append(sections[:0:0], sections...)
	for index := range sections {
		raw, accepted, err := d.readValue("Вес секции «"+displayValue(sections[index].Title)+"»:", false)
		if err != nil || !accepted {
			return err
		}
		sections[index].Weight = &raw
	}
	edit, issues := d.operation.ProposeSectionWeights(snapshot.Token, sections)
	return d.reviewEdit(snapshot, edit, issues)
}
