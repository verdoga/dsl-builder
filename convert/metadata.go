package convert

import (
	"encoding/json"
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// newLesson создаёт только оболочку lesson.Lesson из существующего Document.
// DocumentID уже проверен parsed; переносится точно, без изменения регистра.
// Title=nil или LineCount=nil дают CodeMetadata с именем отсутствующего поля:
// Contract допускает неизвестность, но выход требует конкретного значения.
// Пустой известный Title и известный нулевой LineCount допустимы; заголовок
// не выводится из lines/имени файла, число строк не вычисляется из len(lines).
// Subtitle переносится с отдельным указателем, сохраняя null и пустую строку.
// LineCount переводится из известного model.*int точно в десятичный json.Number,
// без float64 и без дополнительных ограничений поверх существующей модели.
// FormatVersion равен lesson.FormatVersion. Version/CreatedAt оставлены
// revision.Initial; Pages — пустой ненулевой срез до результата structure.Build.
// Processing, Diagnostics и служебные метаданные парсера не копируются.
// При ошибке возвращает nil; исходный Document и его указатели не изменяет.
func newLesson(document model.Document, context diagnostic.Context) (*lesson.Lesson, *diagnostic.Error) {
	if document.Metadata.Title == nil {
		return nil, &diagnostic.Error{
			Code:    CodeMetadata,
			Reason:  "Неизвестно обязательное значение document.metadata.title",
			Context: context,
		}
	}
	if document.LineCount == nil {
		return nil, &diagnostic.Error{
			Code:    CodeMetadata,
			Reason:  "Неизвестно обязательное значение document.lineCount",
			Context: context,
		}
	}
	var subtitle *string
	if document.Metadata.Subtitle != nil {
		value := *document.Metadata.Subtitle
		subtitle = &value
	}
	return &lesson.Lesson{
		FormatVersion: lesson.FormatVersion,
		Document: lesson.Document{
			ID:        *document.Metadata.DocumentID,
			Title:     *document.Metadata.Title,
			Subtitle:  subtitle,
			LineCount: json.Number(strconv.Itoa(*document.LineCount)),
			Pages:     []lesson.Page{},
		},
	}, nil
}
