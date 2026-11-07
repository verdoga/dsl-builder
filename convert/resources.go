package convert

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/resourceembed"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
	"github.com/verdoga/dsl-builder/structure"
)

// prepareResources вызывает resourcefind.Prepare(tree, input.Path,
// input.Result.Document.Metadata.ResourceDirs, context), затем при успехе
// usage.Register(accountedPaths). Возвращает полученный Finder без обёртки.
// Prepare проверяет все явно объявленные каталоги даже без медианод;
// nil/пустые resourceDirs передаются без подстановки области автопоиска.
// Единственное использование filetree — передача того же tree в Prepare:
// convert не обходит каталоги, не нормализует пути и не создаёт Resolver.
func prepareResources(input *parsed.Input, tree *filetree.Resolver, usage *resourceusage.Registry, context diagnostic.Context) (*resourcefind.Finder, *diagnostic.Error) {
	finder, accountedPaths, err := resourcefind.Prepare(tree, input.Path, input.Result.Document.Metadata.ResourceDirs, context)
	if err != nil {
		return nil, err
	}
	usage.Register(accountedPaths)
	return finder, nil
}

// fillMedia координирует mediaSource для binding.Entries[0].Source,
// finder.Find(mediaType, source.Raw, *source.Value, sourceContext) и
// resourceembed.Embed(selection, sourceContext). SourceContext содержит
// исходную строку и source.Start, включая экземпляр развёрнутого фрагмента.
// Привязка содержит только однострочное объявление медиа; постороннее тело
// даёт CodeMediaSource, а не отбрасывается при извлечении первого элемента.
// После Find регистрирует точные selection.AccountedPaths через usage.Register.
// После полного успеха Embed сначала присваивает node.Data, затем вызывает
// usage.MarkUsed(selection.AccountedPaths). Ошибка Find/Embed не отмечается
// как использование; при последующей ошибке Build весь локальный учёт исчезает.
// Порядок/роли файлов и MIME не определяет; данные не дополняет чужим script,
// не объединяет одинаковые байты и не встраивает невыбранные файлы области.
// Прочитанные ресурсы включены в строящийся урок до отметки использования;
// никакие данные/флаги/дети соседних нод не меняются.
func fillMedia(node *lesson.Node, binding structure.Binding, finder *resourcefind.Finder, usage *resourceusage.Registry, context diagnostic.Context) *diagnostic.Error {
	if len(binding.Entries) != 1 || binding.Entries[0] == nil {
		err := &diagnostic.Error{
			Code:    CodeMediaSource,
			Reason:  "Медиа должно содержать только одно однострочное объявление без тела",
			Context: context,
		}
		for _, entry := range binding.Entries {
			if entry == nil {
				continue
			}
			place := context
			place.Line, place.Column = entry.Source.Number, 0
			if len(entry.Source.Elements) != 0 {
				place.Column = entry.Source.Elements[0].Start
			}
			err.Related = append(err.Related, place)
		}
		if len(err.Related) != 0 {
			err.Context = err.Related[0]
		}
		return err
	}
	line := binding.Entries[0].Source
	mediaType, source, err := mediaSource(line, context)
	if err != nil {
		return err
	}
	sourceContext := context
	sourceContext.Line, sourceContext.Column = line.Number, source.Start
	selection, err := finder.Find(mediaType, source.Raw, *source.Value, sourceContext)
	if err != nil {
		return err
	}
	usage.Register(selection.AccountedPaths)
	data, err := resourceembed.Embed(selection, sourceContext)
	if err != nil {
		return err
	}
	node.Data = data
	usage.MarkUsed(selection.AccountedPaths)
	return nil
}

// mediaSource адаптирует одну готовую строку к параметрам Finder.Find:
// требует LineTypeTag, ровно один ElementTypeMediaType и ElementTypeSource
// с известными Value. Их отсутствие/неоднозначность даёт CodeMediaSource
// с исходными координатами и всеми конфликтующими местами в Related.
// mediaType берётся из Value первого элемента, source возвращается как
// существующий model.Element только для чтения; словарь типа проверен parsed.
// Raw/Value/Start сохраняются точно. Кавычки, пути, расширения, подстроки
// и MIME трактует только resourcefind. Пустота, длина, пробелы и числовой
// вид source не получают дополнительных ограничений старого DSL.
func mediaSource(line model.Line, context diagnostic.Context) (mediaType string, source model.Element, err *diagnostic.Error) {
	context.Line, context.Column = line.Number, 0
	var mediaElement model.Element
	mediaCount, sourceCount := 0, 0
	for _, element := range line.Elements {
		switch element.ElementType {
		case model.ElementTypeMediaType:
			mediaCount++
			if mediaCount == 1 {
				mediaElement = element
			}
		case model.ElementTypeSource:
			sourceCount++
			if sourceCount == 1 {
				source = element
			}
		}
	}
	reason := ""
	switch {
	case line.LineType != model.LineTypeTag:
		reason = "Медиа должно быть однострочным объявлением тега"
	case mediaCount != 1 || sourceCount != 1:
		reason = "Объявление медиа должно содержать ровно один тип и один источник"
	case mediaElement.Value == nil || source.Value == nil:
		reason = "Тип и источник медиа должны иметь известные значения"
	}
	if reason != "" {
		err = &diagnostic.Error{Code: CodeMediaSource, Reason: reason, Context: context}
		for _, element := range line.Elements {
			conflict := line.LineType != model.LineTypeTag &&
				(element.ElementType == model.ElementTypeMediaType || element.ElementType == model.ElementTypeSource)
			conflict = conflict || element.ElementType == model.ElementTypeMediaType && (mediaCount != 1 || element.Value == nil)
			conflict = conflict || element.ElementType == model.ElementTypeSource && (sourceCount != 1 || element.Value == nil)
			if conflict {
				place := context
				place.Column = element.Start
				err.Related = append(err.Related, place)
			}
		}
		if len(err.Related) != 0 {
			err.Context = err.Related[0]
		}
		return "", model.Element{}, err
	}
	return *mediaElement.Value, source, nil
}
