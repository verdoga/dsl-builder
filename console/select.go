package console

import "github.com/verdoga/dsl-builder/app"

// chooseLesson перечисляет snapshot.Catalog.Items в текущем порядке:
// техническое имя, ID и название, без содержимого урока. Возвращает точный
// Entry.FileName, а не отображаемый номер. Последний числовой пункт отменяет
// текущий ввод; accepted=false отделён от любого допустимого имени.
func (d *dialog) chooseLesson(snapshot app.Snapshot) (fileName string, accepted bool, err error) {
	var labels []string
	if snapshot.Catalog != nil {
		for _, item := range snapshot.Catalog.Items {
			labels = append(labels, item.Entry.FileName+" | ID: "+item.Entry.ID+" | "+item.Entry.Title)
		}
	}
	labels = append(labels, "Отменить текущий ввод и вернуться")
	selected, err := d.choose("Выберите урок:", labels)
	if err != nil {
		return "", false, err
	}
	if selected == len(labels)-1 {
		return "", false, nil
	}
	return snapshot.Catalog.Items[selected].Entry.FileName, true, nil
}

// chooseSection показывает все точные имена Catalog.Sections и отдельный
// пункт «Без секции», затем отмену. Пустая рабочая секция также доступна.
// Возвращает копию выбранного Title либо nil при явном выборе без секции;
// accepted=false означает отмену. Новое имя вводится отдельным createSection.
func (d *dialog) chooseSection(snapshot app.Snapshot) (section *string, accepted bool, err error) {
	var labels []string
	if snapshot.Catalog != nil {
		for _, section := range snapshot.Catalog.Sections {
			labels = append(labels, section.Title)
		}
	}
	labels = append(labels, "Без секции", "Отменить текущий ввод и вернуться")
	selected, err := d.choose("Выберите секцию:", labels)
	if err != nil {
		return nil, false, err
	}
	if selected == len(labels)-1 {
		return nil, false, nil
	}
	if selected == len(labels)-2 {
		return nil, true, nil
	}
	title := snapshot.Catalog.Sections[selected].Title
	return &title, true, nil
}

// chooseGroup предлагает только существующие непустые группы Catalog.Items,
// следуя Catalog.Sections, затем группу с Section=nil, если такие записи есть.
// При Sectioned=true последняя подписана «Не распределены», иначе «Без секций».
// Нормализации и сортировки предметных значений нет. Отмена отдельным пунктом.
func (d *dialog) chooseGroup(snapshot app.Snapshot) (section *string, accepted bool, err error) {
	var labels []string
	var groups []*string
	if snapshot.Catalog != nil {
		for _, section := range snapshot.Catalog.Sections {
			for _, item := range snapshot.Catalog.Items {
				if item.Section != nil && *item.Section == section.Title {
					title := section.Title
					labels = append(labels, title)
					groups = append(groups, &title)
					break
				}
			}
		}
		for _, item := range snapshot.Catalog.Items {
			if item.Section == nil {
				label := "Без секций"
				if snapshot.Catalog.Sectioned {
					label = "Не распределены"
				}
				labels = append(labels, label)
				groups = append(groups, nil)
				break
			}
		}
	}
	labels = append(labels, "Отменить текущий ввод и вернуться")
	selected, err := d.choose("Выберите группу уроков:", labels)
	if err != nil {
		return nil, false, err
	}
	if selected == len(labels)-1 {
		return nil, false, nil
	}
	return groups[selected], true, nil
}
