// Package compare проектирует содержательные отличия двух снимков урока.
// Версия проекта: 2. Основание: ТЗ 1.0, разделы 7, 12, 18 и 19.
// Вход — lesson.Lesson; parsed, DSL, манифест, ZIP и файловая система
// не обрабатываются. Прямые зависимости: lesson и diagnostic.
// Снимки и результат не изменяются последующими шагами сравнения.
// Пакет не назначает ID, версии, даты и lineCount, не применяет план,
// не получает подтверждений, не пишет превью и не принимает решений о записи.
// app получает отдельные решения по Fields; revision применяет выбранные
// изменения и формирует фактические новые ID для preview. Манифест — catalog.
package compare

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

// Build только координирует validateInputs, compareDocumentFields,
// comparePageSequence, comparePageTitles и comparePages.
// Вызывающий код предварительно находит единственный существующий урок
// по правилам identity.LessonKey; сравнение идентичности уроков не дублируется.
// old и candidate — снимки именно этого урока. После Build вызывающий код
// не изменяет снимки до применения либо повторяет сравнение заново.
// Сначала проверяются обе модели через lesson.Validate; ошибка запрещает
// построение частичного плана. Тогда результат nil и ошибки с контекстом
// соответствующего снимка; успех — ненулевой Plan и пустой срез ошибок.
// Содержательное сравнение исключает Document.ID, Version, CreatedAt,
// LineCount, Page.ID и Node.ID. FormatVersion уже проверяет lesson.
// Совпадение первичных ID кандидата не является основанием сопоставления.
// Отличия только lineCount, ревизии, даты или генерируемых ID дают пустой план.
// Актуальный lineCount переносит revision только при реальном изменении урока.
// Строки и массивы сравниваются точно, без нормализации и переупорядочения.
// Глубина деревьев и величины содержательных данных искусственно не ограничены.
func Build(old *lesson.Lesson, candidate *lesson.Lesson, oldContext diagnostic.Context, candidateContext diagnostic.Context) (*Plan, []*diagnostic.Error) {
	if problems := validateInputs(old, candidate, oldContext, candidateContext); len(problems) > 0 {
		return nil, problems
	}

	plan := &Plan{Fields: compareDocumentFields(old.Document, candidate.Document)}
	matches, reasons := comparePageSequence(old.Document.Pages, candidate.Document.Pages)
	plan.Reasons = reasons
	plan.ReplaceLesson = len(reasons) > 0
	plan.Fields = append(plan.Fields, comparePageTitles(old.Document.Pages, candidate.Document.Pages, matches, plan.ReplaceLesson)...)
	if !plan.ReplaceLesson {
		plan.Pages = comparePages(old.Document.Pages, candidate.Document.Pages)
	}
	return plan, nil
}

// validateInputs вызывает lesson.Validate для каждого снимка, не повторяя
// схему и синтаксис ID. Ошибки старого снимка предшествуют ошибкам кандидата;
// исходные коды diagnostic сохраняются. nil обрабатывает lesson.Validate.
func validateInputs(old *lesson.Lesson, candidate *lesson.Lesson, oldContext diagnostic.Context, candidateContext diagnostic.Context) []*diagnostic.Error {
	problems := lesson.Validate(old, oldContext)
	candidateProblems := lesson.Validate(candidate, candidateContext)
	return append(problems, candidateProblems...)
}
