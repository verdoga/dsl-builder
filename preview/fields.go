package preview

import (
	"fmt"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/module"
	"strings"
)

// FieldProposal показывает одно предложение поля перед отдельным решением:
// техническое имя, ID урока, поле, известную старую страницу, позицию новой
// страницы с 1, «Было» и «Станет». before — старый File, не кандидат.
// Для PageTitle и OldPage=-1 пишет «прежнее поле не установлено»;
// Before=nil в этом случае не выдаёт за известный прежний null.
// Явно показывает результат отказа: новая страница останется без названия
// (title=null). В остальных случаях отказ сохраняет известное Before.
// Даже After=nil не скрывает предложение несопоставленной страницы.
// Вопрос, варианты ответа и получение решения принадлежат console.
func FieldProposal(before *module.File, change compare.FieldChange) (string, error) {
	if before == nil {
		return "", fmt.Errorf("Невозможно показать предложение поля: отсутствует прежний файл.")
	}
	old := before.Lesson()
	if old == nil {
		return "", fmt.Errorf("Невозможно показать предложение поля: отсутствует прежний снимок урока.")
	}
	switch change.Field {
	case compare.DocumentTitle, compare.DocumentSubtitle:
		if change.OldPage != -1 || change.NewPage != -1 {
			return "", fmt.Errorf("Невозможно показать поле документа: несогласованные индексы страниц.")
		}
		if change.Field == compare.DocumentTitle && (change.Before == nil || change.After == nil) {
			return "", fmt.Errorf("Невозможно показать название урока: не задано обязательное значение.")
		}
	case compare.PageTitle:
		if change.OldPage < -1 || change.NewPage < 0 || change.OldPage >= len(old.Document.Pages) {
			return "", fmt.Errorf("Невозможно показать название страницы: недоступен адрес страницы.")
		}
		if change.OldPage == -1 && change.Before != nil {
			return "", fmt.Errorf("Невозможно показать название страницы: прежнее поле не установлено, но задано его значение.")
		}
	default:
		return "", fmt.Errorf("Невозможно показать предложение: неизвестный селектор поля.")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Файл: %s\nID урока: %s\nПоле: %s\n", quotedText(before.FileName()), quotedText(old.Document.ID), quotedText(string(change.Field)))
	if change.Field == compare.PageTitle {
		if change.OldPage >= 0 {
			page := old.Document.Pages[change.OldPage]
			fmt.Fprintf(&out, "Прежняя страница: %s; название: %s\n", quotedText(page.ID), optionalText(page.Title))
		}
		fmt.Fprintf(&out, "Позиция новой страницы: %d\n", uint(change.NewPage)+1)
	}
	out.WriteString(fieldValues(change))
	if change.Field == compare.PageTitle && change.OldPage == -1 {
		out.WriteString("При отказе новая страница останется без названия (title=null).\n")
	} else {
		fmt.Fprintf(&out, "При отказе будет сохранено прежнее значение: %s\n", optionalText(change.Before))
	}
	return out.String(), nil
}

// AcceptedFields перечисляет только подтверждённые вызывающим кодом изменения
// полей, в переданном порядке. accepted — выбранные элементы Plan.Fields;
// список нельзя автоматически заменять всеми предложениями плана.
// Показывает точные Before/After и страницу по NewPage в итоговом after;
// для известного OldPage указывает прежнюю страницу из before.
// При полной замене адреса обозначают происхождение названия, не перенос ID.
// Пустой список явно обозначает отсутствие подтверждённых изменений полей,
// но не отсутствие изменений содержимого или манифеста.
func AcceptedFields(before *module.File, after *module.File, accepted []compare.FieldChange) (string, error) {
	if before == nil || after == nil {
		return "", fmt.Errorf("Невозможно показать подтверждённые поля: отсутствует обязательный файл.")
	}
	old, result := before.Lesson(), after.Lesson()
	if old == nil || result == nil {
		return "", fmt.Errorf("Невозможно показать подтверждённые поля: отсутствует обязательный снимок урока.")
	}
	if before.FileName() != after.FileName() || old.Document.ID != result.Document.ID {
		return "", fmt.Errorf("Невозможно показать подтверждённые поля: снимки относятся к разным файлам или урокам.")
	}
	if len(accepted) == 0 {
		return "Подтверждённых изменений полей нет.\n", nil
	}
	var out strings.Builder
	out.WriteString("Подтверждённые изменения полей:\n")
	for _, change := range accepted {
		switch change.Field {
		case compare.DocumentTitle, compare.DocumentSubtitle:
			if change.OldPage != -1 || change.NewPage != -1 {
				return "", fmt.Errorf("Невозможно показать поле документа: несогласованные индексы страниц.")
			}
			if change.Field == compare.DocumentTitle && (change.Before == nil || change.After == nil) {
				return "", fmt.Errorf("Невозможно показать название урока: не задано обязательное значение.")
			}
		case compare.PageTitle:
			if change.OldPage < -1 || change.OldPage >= len(old.Document.Pages) || change.NewPage < 0 || change.NewPage >= len(result.Document.Pages) {
				return "", fmt.Errorf("Невозможно показать подтверждённое название страницы: недоступен адрес страницы.")
			}
			if change.OldPage == -1 && change.Before != nil {
				return "", fmt.Errorf("Невозможно показать название страницы: прежнее поле не установлено, но задано его значение.")
			}
		default:
			return "", fmt.Errorf("Невозможно показать подтверждённое изменение: неизвестный селектор поля.")
		}
		fmt.Fprintf(&out, "%s:\n", quotedText(string(change.Field)))
		if change.Field == compare.PageTitle {
			if change.OldPage >= 0 {
				page := old.Document.Pages[change.OldPage]
				fmt.Fprintf(&out, "Прежняя страница — источник названия: %s; название: %s\n", quotedText(page.ID), optionalText(page.Title))
			}
			page := result.Document.Pages[change.NewPage]
			fmt.Fprintf(&out, "Итоговая страница (позиция %d): %s; название: %s\n", uint(change.NewPage)+1, quotedText(page.ID), optionalText(page.Title))
			out.WriteString("Адреса страниц обозначают происхождение названия; переноса ID не предполагается.\n")
		}
		out.WriteString(fieldValues(change))
	}
	return out.String(), nil
}

// fieldValues представляет одно поле: различает неизвестное прежнее поле,
// null, пустую строку и непустую строку, сохраняя регистр и пробелы.
func fieldValues(change compare.FieldChange) string {
	before := optionalText(change.Before)
	if change.Field == compare.PageTitle && change.OldPage == -1 {
		before = "прежнее поле не установлено"
	}
	return "Было: " + before + "\nСтанет: " + optionalText(change.After) + "\n"
}
