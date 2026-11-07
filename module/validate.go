package module

import (
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
)

// Validate — координатор проверки готового состава без его изменения.
// nil Module даёт CodeModule, пустые Files — CodeEmpty. Вызывает validateFiles,
// manifest.Validate и, только после успешной схемы, catalog.Import.
// Import проверяет размещение у владельца и возвращает плоские Items для
// ValidateLinks; catalog.ValidateManifest повторно не вызывается.
// Ненулевой State с ошибками размещения всё равно позволяет проверить связи;
// ошибка одной ветви не скрывает независимые ошибки других файлов.
// ValidateLinks вызывается только после успешной validateFiles и при ненулевом
// State: неверные числа и неоднозначные файлы не передаются сравнению метаданных.
// До этого moduleio проверяет форму прочитанного JSON средствами владельцев
// lesson.ValidateJSON/manifest.ValidateShape, ZIP-записи и один manifest.json.
// Порядок ошибок: файлы, схема манифеста, размещение, связи. Любая ошибка
// запрещает запись/успешное чтение модуля; исправления и пересортировка отсутствуют.
func Validate(value *Module, context diagnostic.Context) []*diagnostic.Error {
	if value == nil {
		return []*diagnostic.Error{{Code: CodeModule, Reason: "Отсутствует модель модуля.", Context: context}}
	}
	var fileProblems []*diagnostic.Error
	if len(value.Files) == 0 {
		fileProblems = append(fileProblems, &diagnostic.Error{Code: CodeEmpty, Reason: "Модуль не содержит файлов уроков.", Context: context})
	}
	fileProblems = append(fileProblems, validateFiles(value.Files, context)...)
	problems := fileProblems
	manifestContext := context
	manifestContext.FileName = ManifestFileName
	schemaProblems := manifest.Validate(value.Manifest, manifestContext)
	problems = append(problems, schemaProblems...)
	if len(schemaProblems) != 0 {
		return problems
	}
	state, placementProblems := catalog.Import(value.Manifest, manifestContext)
	problems = append(problems, placementProblems...)
	if len(fileProblems) == 0 && state != nil {
		problems = append(problems, ValidateLinks(value.Files, state.Items, context)...)
	}
	return problems
}

// validateFiles координирует validateFileNames, lesson.Validate для каждого
// ненулевого файла и validateIDs по ID уроков. Не повторяет проверки дерева
// или встроенных ресурсов. Сохраняет контекст каждого источника; при его
// отсутствии использует переданный архив, техническое имя и известный ID.
func validateFiles(files []*File, context diagnostic.Context) []*diagnostic.Error {
	problems := validateFileNames(files, context)
	var identities []diagnostic.Context
	for _, file := range files {
		if file == nil {
			continue // Отсутствующий файл уже сообщён validateFileNames.
		}
		source := file.Context()
		if source.Path == "" {
			source.Path = context.Path
		}
		source.FileName = file.name
		if file.value == nil {
			problems = append(problems, &diagnostic.Error{Code: CodeFile, Reason: "Отсутствует снимок урока в файле.", Context: source})
		} else {
			source.LessonID = file.value.Document.ID
			identities = append(identities, source)
		}
		problems = append(problems, lesson.Validate(file.value, source)...)
	}
	return append(problems, validateIDs(identities)...)
}

// ValidateLinks проверяет только состав и согласованность связей готовых
// файлов с catalog.Items. Применим и до Export для рабочего плана; не заменяет
// catalog.Validate/Export, проверки схем и итоговый Validate перед записью.
// Предусловие: validateFiles успешна, Entry проверены manifest по схеме;
// отдельные повторы ID/имён записей ещё допустимы и проверяются здесь.
// Координирует validateEntryKeys, validateCoverage и validateEntryMetadata
// для однозначных пар. Ни порядок Files, ни положение секции не являются ID.
func ValidateLinks(files []*File, items []catalog.Item, context diagnostic.Context) []*diagnostic.Error {
	problems := validateEntryKeys(items, context)
	problems = append(problems, validateCoverage(files, items, context)...)
	byName := make(map[string][]*File, len(files))
	for _, file := range files {
		if file != nil {
			byName[file.name] = append(byName[file.name], file)
		}
	}
	references := make(map[string]int, len(items))
	for _, item := range items {
		references[item.Entry.FileName]++
	}
	for _, item := range items {
		matches := byName[item.Entry.FileName]
		if len(matches) != 1 || references[item.Entry.FileName] != 1 {
			continue
		}
		if item.Context.Path == "" {
			item.Context.Path = context.Path
		}
		item.Context.FileName = item.Entry.FileName
		item.Context.LessonID = item.Entry.ID
		file := *matches[0]
		if file.context.Path == "" {
			file.context.Path = context.Path
		}
		problems = append(problems, validateEntryMetadata(&file, item)...)
	}
	return problems
}

// validateEntryKeys проверяет независимую уникальность Entry.ID через
// validateIDs и Entry.FileName точным равенством по всему списку Items,
// включая разные секции. uniqueItems схемы этого не заменяет.
// Для повторного fileName сообщает CodeDuplicateReference со всеми записями.
func validateEntryKeys(items []catalog.Item, context diagnostic.Context) []*diagnostic.Error {
	contexts := make([]diagnostic.Context, len(items))
	groups := make(map[string][]diagnostic.Context, len(items))
	for index, item := range items {
		source := item.Context
		if source.Path == "" {
			source.Path = context.Path
		}
		source.FileName = item.Entry.FileName
		source.LessonID = item.Entry.ID
		contexts[index] = source
		groups[item.Entry.FileName] = append(groups[item.Entry.FileName], source)
	}
	problems := validateIDs(contexts)
	seen := make(map[string]bool, len(groups))
	for index, item := range items {
		name := item.Entry.FileName
		if seen[name] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeDuplicateReference, Reason: "Несколько записей манифеста ссылаются на одно имя файла.",
				Context: contexts[index], Related: append([]diagnostic.Context(nil), groups[name]...),
				Details: []diagnostic.Detail{{Label: "Поле", Value: "fileName"}, {Label: "Техническое имя", Value: name}},
			})
		}
		seen[name] = true
	}
	return problems
}

// validateCoverage проверяет двустороннее соответствие точных имён:
// каждая запись ссылается на один файл, каждый файл имеет одну запись.
// Отсутствующий файл — CodeMissingFile; файл без записи — CodeUnlistedFile.
// Повторы не схлопываются в map и не дают произвольной пары для метаданных;
// их сообщает validateFileNames/validateEntryKeys. Порядок: Items, затем Files.
func validateCoverage(files []*File, items []catalog.Item, context diagnostic.Context) []*diagnostic.Error {
	fileCounts := make(map[string]int, len(files))
	for _, file := range files {
		if file != nil {
			fileCounts[file.name]++
		}
	}
	entryCounts := make(map[string]int, len(items))
	var problems []*diagnostic.Error
	for _, item := range items {
		name := item.Entry.FileName
		entryCounts[name]++
		if fileCounts[name] != 0 {
			continue
		}
		source := item.Context
		if source.Path == "" {
			source.Path = context.Path
		}
		source.FileName = name
		source.LessonID = item.Entry.ID
		problems = append(problems, &diagnostic.Error{
			Code: CodeMissingFile, Reason: "Запись манифеста ссылается на отсутствующий файл урока.", Context: source,
			Details: []diagnostic.Detail{{Label: "Поле", Value: "fileName"}, {Label: "Техническое имя", Value: name}},
		})
	}
	for _, file := range files {
		if file == nil || entryCounts[file.name] != 0 {
			continue
		}
		source := file.Context()
		if source.Path == "" {
			source.Path = context.Path
		}
		problems = append(problems, &diagnostic.Error{
			Code: CodeUnlistedFile, Reason: "Файл урока отсутствует в записях манифеста.", Context: source,
			Details: []diagnostic.Detail{{Label: "Поле", Value: "fileName"}, {Label: "Техническое имя", Value: file.name}},
		})
	}
	return problems
}
