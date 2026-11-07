// Package parsed загружает один технический JSON Contract 1.0 в существующий
// parsed/model и готовит индексы для преобразования. Исходники не изменяются.
// Проверки ограничены ТЗ и Contract: DSL уже разобран и проверен upstream.
// Пакет не разбирает TXT, не строит урок, не разрешает ресурсы и фрагменты,
// не проверяет состав модуля, совпадения ID между файлами или веса групп.
// Дополнительные ограничения DSL на ID, текст и source не вводятся.
// Используются только стандартная библиотека и объявленные зависимости.
package parsed

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// Input связывает проверенный результат с адресом технического JSON и индексом.
// Это рабочий вход конвейера, а не копия модели и не новый JSON-контракт.
// Получают только через Load; поля и вложенные данные после загрузки читают
// без изменения, иначе индексы и факт проверки становятся недействительными.
type Input struct {
	// Path — полный путь обращения к техническому JSON, переданный в Load.
	// Его каталог является базой ресурсов; document.filePath её не заменяет.
	Path string

	// Result — существующая модель с исходными значениями, порядком и null.
	// Warning и recommendation сохранены как входные данные, но не выводятся.
	Result model.Result

	// Index — номера строк и непосредственные связи исходного документа.
	Index LineIndex
}

// Load читает один полный путь, подготовленный вызывающим кодом через filetree.
// Координирует readBytes, prepareUTF8, decodeResult, parserErrors,
// validateContract и подготовку индекса. Каждый этап реализуется отдельно.
// Признаки ошибок парсера проверяются сразу после декодирования формы,
// до проверки согласованности флагов и ссылок: один признак уже блокирует вход.
// После чтения metadata контекст дополняется известным исходным documentId,
// без генерации ID; Context.Path всегда сохраняет переданный path.
// На любой ошибке возвращает nil и непустой список диагностик, без частичного
// пригодного Input; при успехе — Input и nil. Ничего не печатает и не записывает.
// Ошибка обязательного файла должна блокировать всю операцию у вызывающего app.
func Load(path string) (*Input, []*diagnostic.Error) {
	data, problems := readBytes(path)
	if len(problems) != 0 {
		return nil, problems
	}
	context := diagnostic.Context{Path: path}
	data, problem := prepareUTF8(data, context)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	result, problem := decodeResult(data, context)
	if result.Document.Metadata.DocumentID != nil {
		context.LessonID = *result.Document.Metadata.DocumentID
	}
	if problem != nil {
		problem.Context.Path = path
		if result.Document.Metadata.DocumentID != nil {
			problem.Context.LessonID = context.LessonID
		}
		return nil, []*diagnostic.Error{problem}
	}
	if problems := parserErrors(&result, context); len(problems) != 0 {
		return nil, problems
	}
	index, problems := validateContract(&result, context)
	if len(problems) != 0 {
		return nil, problems
	}
	return &Input{Path: path, Result: result, Index: index}, nil
}
