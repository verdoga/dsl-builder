package app

import (
	"path/filepath"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/resourceusage"
)

// Analyze — короткая последовательность loadBaseline, loadSources,
// prepareIncoming, convertSources и prepareScenario; последующий этап только
// после успеха предыдущего. Сначала создаёт новый общий Resolver и Registry,
// фиксирует at до convert/revision. Готовность analyzed ставится только после
// всех этапов; ненулевой catalog с исправляемыми блокировками завершает анализ.
// Ошибка входа блокирует всю операцию; каталог с исправляемыми ошибками
// после prepareScenario остаётся доступен через Snapshot для редактирования.
// Повторный вызов требует Retry: старый план и новые входы не смешиваются.
func (op *Operation) Analyze(at time.Time) []*diagnostic.Error {
	if op == nil || op.key == nil || op.key.owner != op || op.finished || op.tree != nil || op.analyzed {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Для анализа нужна новая операция либо повтор через Retry."}}
	}
	if op.mode != Create && op.mode != Update && op.mode != Add {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Не выбран допустимый режим анализа."}}
	}
	op.tree, op.usage, op.at = &filetree.Resolver{}, &resourceusage.Registry{}, at
	for _, stage := range []func() []*diagnostic.Error{op.loadBaseline, op.loadSources, op.prepareIncoming, op.convertSources} {
		if issues := stage(); len(issues) != 0 {
			op.issues = issues
			return cloneDiagnostics(issues)
		}
	}
	issues := op.prepareScenario()
	op.issues = issues
	op.analyzed = op.current != nil && op.placement != nil && (op.mode != Update || op.plan != nil && len(issues) == 0)
	return cloneDiagnostics(issues)
}

// loadBaseline для Update/Add вызывает moduleio.Read и catalog.Import.
// Read с ошибкой всегда даёт остановку; Import с nil не позволяет продолжить,
// ненулевой State и ошибки сохраняются раздельно. Создание пропускает чтение.
// До чтения сохраняет inspectDestination для исходного ZIP, после чтения
// проверяет то же наблюдение через checkDestination. Оно остаётся опорой
// для окончательного согласия: новый осмотр не узаконивает чужую замену ZIP.
// baseline и beforeCatalog впоследствии не изменяются; Files сохраняют байты.
func (op *Operation) loadBaseline() []*diagnostic.Error {
	if op.mode == Create {
		return nil
	}
	target, problem := inspectDestination(op.archivePath)
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	op.target = target
	value, issues := moduleio.Read(op.archivePath)
	if len(issues) != 0 {
		return issues
	}
	if problem := op.checkDestination(); problem != nil {
		return []*diagnostic.Error{problem}
	}
	before, issues := catalog.Import(value.Manifest, diagnostic.Context{Path: op.archivePath, FileName: module.ManifestFileName})
	if before == nil {
		if len(issues) == 0 {
			issues = []*diagnostic.Error{{Code: CodeState, Reason: "Не удалось получить исходное размещение архива.", Context: diagnostic.Context{Path: op.archivePath}}}
		}
		return issues
	}
	op.baseline, op.beforeCatalog, op.issues = value, before, issues
	return nil
}

// loadSources координирует discoverSources и parsed.Load каждого найденного
// Entry.Path. Ни один JSON не исключается по содержимому; любая ошибка
// запрещает публикацию частичного состава. Входы впоследствии не изменяются.
func (op *Operation) loadSources() []*diagnostic.Error {
	if issues := op.discoverSources(); len(issues) != 0 {
		return issues
	}
	inputs := make([]*parsed.Input, len(op.entries))
	var issues []*diagnostic.Error
	for i, entry := range op.entries {
		input, problems := parsed.Load(entry.Path)
		issues = append(issues, problems...)
		inputs[i] = input
	}
	if len(issues) != 0 {
		return issues
	}
	op.inputs = inputs
	return nil
}

// discoverSources разрешает путь через filetree.Resolve и общий tree.Inspect.
// Create вызывает JSONFiles, Update только JSONFile, Add выбирает по виду.
// Каталог сортируется filetree.SortRelative от выбранного каталога;
// одиночный файл имеет базу filepath.Dir(Path). Возвращаемый порядок стабилен.
func (op *Operation) discoverSources() []*diagnostic.Error {
	path, problem := filetree.Resolve(op.baseDir, op.sourcePath)
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	entry, problem := op.tree.Inspect(path)
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	var entries []filetree.Entry
	base := path
	if op.mode == Create || op.mode == Add && entry.Info.IsDir() {
		entries, problem = op.tree.JSONFiles(path)
	} else {
		entry, problem = op.tree.JSONFile(path)
		entries, base = []filetree.Entry{entry}, filepath.Dir(path)
	}
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	entries, problem = filetree.SortRelative(base, entries)
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	op.entries = entries
	return nil
}

// prepareIncoming вызывает module.CheckNewIDs для Create/Add либо
// module.FindByID для Update. Для Create/Add затем module.AssignFileNames
// до convert; существующие имена не меняются, пробелы нумерации не занимаются.
// Для Update names содержит старое FileName, oldFile сохраняется целиком.
// Затем validateInitialPlacement вызывается только при Create.
func (op *Operation) prepareIncoming() []*diagnostic.Error {
	contexts := make([]diagnostic.Context, len(op.inputs))
	for i, input := range op.inputs {
		contexts[i] = diagnostic.Context{Path: input.Path, LessonID: *input.Result.Document.Metadata.DocumentID}
	}
	if op.mode == Update {
		file, issues := module.FindByID(op.baseline.Files, contexts[0].LessonID, contexts[0])
		if len(issues) != 0 {
			return issues
		}
		op.oldFile, op.names = file, []string{file.FileName()}
		return nil
	}
	var existing []*module.File
	if op.baseline != nil {
		existing = op.baseline.Files
	}
	if issues := module.CheckNewIDs(contexts, existing); len(issues) != 0 {
		return issues
	}
	names, issues := module.AssignFileNames(existing, len(op.inputs), diagnostic.Context{Path: op.sourcePath})
	if len(issues) != 0 {
		return issues
	}
	op.names = names
	if op.mode == Create {
		return op.validateInitialPlacement()
	}
	return nil
}

// validateInitialPlacement строит частичные catalog.Item из проверенных входов
// и вызывает catalog.ValidateNewPlacement до convert: заполнены ID, FileName,
// Order, Section, RelativePath через filetree.Relative и Context.
// Схему полной Entry здесь не проверяют и отсутствующие поля не выдумывают.
func (op *Operation) validateInitialPlacement() []*diagnostic.Error {
	items := make([]catalog.Item, len(op.inputs))
	for i, input := range op.inputs {
		relative, problem := filetree.Relative(op.sourcePath, input.Path)
		if problem != nil {
			return []*diagnostic.Error{problem}
		}
		metadata := input.Result.Document.Metadata
		items[i].Entry.ID, items[i].Entry.FileName, items[i].Entry.Order = *metadata.DocumentID, op.names[i], metadata.Order
		items[i].Section, items[i].RelativePath = metadata.Section, relative
		items[i].Context = diagnostic.Context{Path: input.Path, FileName: op.names[i], LessonID: *metadata.DocumentID}
	}
	return catalog.ValidateNewPlacement(items)
}

// convertSources последовательно вызывает convertSource для каждого входа.
// Локальные Registry объединяет через op.usage.Merge только после успеха;
// при любой ошибке готовый частичный набор не становится current.
func (op *Operation) convertSources() []*diagnostic.Error {
	converted := make([]*module.File, len(op.inputs))
	for i, input := range op.inputs {
		file, issues := op.convertSource(input, op.names[i])
		if len(issues) != 0 {
			return issues
		}
		converted[i] = file
	}
	op.converted = converted
	return nil
}

// convertSource вызывает convert.Build(input, op.tree, op.at, context), затем
// module.NewFile с заранее назначенным именем. Успешный Registry передаёт Merge.
// context содержит Path, FileName и исходный DocumentID; lineCount не вычисляется.
func (op *Operation) convertSource(input *parsed.Input, fileName string) (*module.File, []*diagnostic.Error) {
	context := diagnostic.Context{Path: input.Path, FileName: fileName, LessonID: *input.Result.Document.Metadata.DocumentID}
	value, usage, issues := convert.Build(input, op.tree, op.at, context)
	if len(issues) != 0 {
		return nil, issues
	}
	file, issues := module.NewFile(fileName, value, context)
	if len(issues) != 0 {
		return nil, issues
	}
	op.usage.Merge(usage)
	return file, nil
}

// prepareScenario выбирает createCatalog, addCatalog либо compareUpdate,
// затем refreshPlan. Здесь только последовательность, без правил предмета.
func (op *Operation) prepareScenario() []*diagnostic.Error {
	var issues []*diagnostic.Error
	switch op.mode {
	case Create:
		issues = op.createCatalog()
	case Add:
		issues = op.addCatalog()
	case Update:
		issues = op.compareUpdate()
		if len(issues) != 0 {
			return issues
		}
	}
	if op.placement == nil {
		return issues
	}
	return op.refreshPlan()
}

// incomingItems связывает готовые File.Lesson().Document через
// catalog.EntryFromLesson с Section/Order соответствующих parsed.Input.
// RelativePath берёт filetree.Relative; порядок совпадает с отсортированным входом.
func (op *Operation) incomingItems() ([]catalog.Item, []*diagnostic.Error) {
	base := op.sourcePath
	if len(op.inputs) == 1 && op.inputs[0].Path == base {
		base = filepath.Dir(base)
	}
	items := make([]catalog.Item, len(op.inputs))
	for i, input := range op.inputs {
		relative, problem := filetree.Relative(base, input.Path)
		if problem != nil {
			return nil, []*diagnostic.Error{problem}
		}
		metadata, file := input.Result.Document.Metadata, op.converted[i]
		items[i] = catalog.Item{Entry: catalog.EntryFromLesson(file.Lesson().Document, file.FileName(), metadata.Order), Section: metadata.Section, RelativePath: relative, Context: file.Context()}
	}
	return items, nil
}

// createCatalog вызывает incomingItems и catalog.Create с введёнными названиями.
// nil State означает провал анализа. Ненулевой State с конфликтами сохраняет
// все converted в current.Files для превью и правок, даже до успешного Export.
func (op *Operation) createCatalog() []*diagnostic.Error {
	items, issues := op.incomingItems()
	if len(issues) != 0 {
		return issues
	}
	value, issues := catalog.Create(op.courseTitle, op.unitTitle, items)
	if value == nil {
		return issues
	}
	op.placement = value
	op.current = &module.Module{Files: append([]*module.File(nil), op.converted...)}
	op.lessonChanged, op.revisionReady = true, true
	return issues
}

// addCatalog вызывает incomingItems и catalog.Add над Clone(beforeCatalog).
// При ненулевом результате current.Files — новый срез baseline.Files плюс
// converted: прежние File используются целиком, без NewFile/повторного convert.
// Отсутствующие веса, смешение и незавершённые секции остаются блокировками.
func (op *Operation) addCatalog() []*diagnostic.Error {
	items, issues := op.incomingItems()
	if len(issues) != 0 {
		return issues
	}
	value, issues := catalog.Add(catalog.Clone(op.beforeCatalog), items)
	if value == nil {
		return issues
	}
	op.placement, op.current = value, cloneModule(op.baseline)
	op.current.Files = append(op.current.Files, op.converted...)
	op.lessonChanged, op.revisionReady = true, true
	return issues
}
