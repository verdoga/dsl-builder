package preview

import (
	"errors"
	"fmt"
	"strings"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/module"
)

// UpdateHeading выводит путь архива, техническое имя, стабильный ID,
// итоговое название, присутствующий подзаголовок и старую/итоговую ревизии.
// before и after — файлы одного урока из старого и подготовленного модуля.
// Читает Version буквально; не увеличивает её и не формирует суффиксы.
// Переход может сохранять версию при изменениях только манифеста.
func UpdateHeading(archivePath string, before *module.File, after *module.File) (string, error) {
	if before == nil || after == nil {
		return "", errors.New("Для показа обновления нужны прежний и итоговый файлы урока")
	}
	old, result := before.Lesson(), after.Lesson()
	if old == nil || result == nil {
		return "", errors.New("Для показа обновления нужны прежний и итоговый снимки урока")
	}
	if before.FileName() != after.FileName() || old.Document.ID != result.Document.ID {
		return "", errors.New("Снимки для показа обновления относятся к разным файлам или урокам")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "ПРЕДВАРИТЕЛЬНЫЙ ПРОСМОТР ОБНОВЛЕНИЯ\n\nАрхив: %s\nФайл урока: %s\nID урока: %s\nНазвание: %s\n",
		quotedText(archivePath), quotedText(after.FileName()), quotedText(result.Document.ID), quotedText(result.Document.Title))
	if result.Document.Subtitle != nil {
		fmt.Fprintf(&out, "Подзаголовок: %s\n", optionalText(result.Document.Subtitle))
	}
	fmt.Fprintf(&out, "Версия урока: %s -> %s\n", old.Document.Version, result.Document.Version)
	return out.String(), nil
}

// Replacements представляет только уровень и состав замен из готового plan.
// after содержит итог применения этого плана и решений по полям, включая
// рассчитанные владельцем ревизии ID; первичный кандидат сюда не передаётся.
// Не сравнивает модели повторно и не использует Plan.Fields как подтверждения.
// Небольшой координатор lessonReplacement либо pageReplacements.
// При пустом плане сообщает лишь «Замены нод не запланированы»;
// решение об отсутствии всей операции остаётся у вызывающего кода.
func Replacements(before *module.File, after *module.File, plan *compare.Plan) (string, error) {
	if before == nil || after == nil || plan == nil {
		return "", errors.New("Для показа замен нужны прежний и итоговый файлы урока и план")
	}
	old, result := before.Lesson(), after.Lesson()
	if old == nil || result == nil {
		return "", errors.New("Для показа замен нужны прежний и итоговый снимки урока")
	}
	if before.FileName() != after.FileName() || old.Document.ID != result.Document.ID {
		return "", errors.New("Снимки для показа замен относятся к разным файлам или урокам")
	}
	if plan.ReplaceLesson {
		if len(plan.Pages) != 0 || len(plan.Reasons) == 0 {
			return "", errors.New("Селектор полной замены урока не согласован")
		}
		return lessonReplacement(before, after, plan.Reasons)
	}
	if len(plan.Reasons) != 0 {
		return "", errors.New("Причины полной замены урока не согласованы с селектором")
	}
	return pageReplacements(before, after, plan.Pages)
}

// pageReplacements представляет переданные изменённые страницы по порядку,
// вызывая pageReplacement либо nodeReplacement для каждого готового изменения.
// Собственное отличие ребёнка не добавляет родителя в перечень замен.
func pageReplacements(before *module.File, after *module.File, changes []compare.PageChange) (string, error) {
	if before == nil || after == nil {
		return "", errors.New("Для показа замен страниц нужны прежний и итоговый файлы урока")
	}
	old, result := before.Lesson(), after.Lesson()
	if old == nil || result == nil {
		return "", errors.New("Для показа замен страниц нужны прежний и итоговый снимки урока")
	}
	if before.FileName() != after.FileName() || old.Document.ID != result.Document.ID {
		return "", errors.New("Снимки для показа замен страниц относятся к разным файлам или урокам")
	}
	if len(changes) == 0 {
		return "Замены нод не запланированы\n", nil
	}
	var out strings.Builder
	for _, change := range changes {
		if change.Index < 0 || change.Index >= len(old.Document.Pages) || change.Index >= len(result.Document.Pages) {
			return "", errors.New("Страница для показа замен недоступна")
		}
		if change.ReplaceNodes {
			if len(change.Nodes) != 0 || len(change.Reasons) == 0 {
				return "", errors.New("Селектор полной замены нод страницы не согласован")
			}
			block, err := pageReplacement(before, after, change)
			if err != nil {
				return "", err
			}
			out.WriteString(block)
			continue
		}
		if len(change.Reasons) != 0 || len(change.Nodes) == 0 {
			return "", errors.New("Селектор отдельных замен нод страницы не согласован")
		}
		for _, node := range change.Nodes {
			block, err := nodeReplacement(before, after, change.Index, node)
			if err != nil {
				return "", err
			}
			out.WriteString(block)
		}
	}
	return out.String(), nil
}
