package app

import (
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/resourceusage"
	"github.com/verdoga/dsl-builder/revision"
	"github.com/verdoga/dsl-builder/safewrite"
)

// Mode определяет один из трёх сценариев, без командного диспетчера.
type Mode string

const (
	// Create — создание из каталога.
	Create Mode = "create"
	// Update — обновление одного урока.
	Update Mode = "update"
	// Add — добавление файла либо каталога.
	Add Mode = "add"
)

// Operation владеет одним анализом, решениями и единственной попыткой записи.
// Публичные методы проверяют применимость до изменения закрытого состояния.
type Operation struct {
	// mode — выбранный сценарий.
	mode Mode
	// baseDir — явная полная база интерактивных относительных путей.
	baseDir string
	// sourcePath — выбранный путь исходников; сохраняется для Retry.
	sourcePath string
	// archivePath — исходный ZIP; пуст при создании.
	archivePath string
	// courseTitle — первоначальное название для создания и повторного анализа.
	courseTitle string
	// unitTitle — первоначальный юнит; пустая строка означает отсутствие.
	unitTitle string
	// at — стабильное время преобразования и ревизии текущего анализа.
	at time.Time
	// tree — один Resolver на весь анализ, включая все convert.Build.
	tree *filetree.Resolver
	// inputs — проверенные parsed.Input; наружу изменяемые входы не выдаются.
	inputs []*parsed.Input
	// entries — найденные JSON в детерминированном порядке.
	entries []filetree.Entry
	// names — заранее назначенные имена, соответствующие entries/inputs.
	names []string
	// baseline — неизменный загруженный Module; nil для создания.
	baseline *module.Module
	// beforeCatalog — неизменное размещение до операции; для создания nil.
	beforeCatalog *catalog.State
	// current — полный состав с готовыми Files; Manifest может ждать Export.
	current *module.Module
	// manifestReady — текущий Manifest успешно экспортирован и проверен.
	manifestReady bool
	// placement — принятое размещение, включая исправляемые блокировки.
	placement *catalog.State
	// converted — готовые новые файлы либо единственный кандидат обновления.
	converted []*module.File
	// usage — объединённый реестр только обрабатываемых исходников.
	usage *resourceusage.Registry
	// oldFile — стабильный прежний файл обновляемого урока, включая OriginalBytes.
	oldFile *module.File
	// newFile — стабильный первичный кандидат для compare и revision.
	newFile *module.File
	// plan — полный неизменённый compare.Plan для oldFile/newFile.
	plan *compare.Plan
	// fields — решения по всем plan.Fields; пустое значение означает ожидание.
	fields []revision.Decision
	// proposals — исходные отдельные изменения section/order из ProposePlacement.
	proposals []catalog.Change
	// placementDecisions — отдельные решения по proposals в том же порядке.
	placementDecisions []revision.Decision
	// revised — итоговый файл после PrepareRevision; до него равен oldFile.
	revised *module.File
	// lessonChanged — только результат revision.Apply либо наличие новых уроков.
	lessonChanged bool
	// analyzed — анализ целиком завершён, исходные ошибки отсутствуют.
	analyzed bool
	// revisionReady — все поля решены и итоговая ревизия подготовлена.
	revisionReady bool
	// issues — актуальные блокировки; ошибки ввода правки сюда не подмешиваются.
	issues []*diagnostic.Error
	// key — идентичность текущего принятого снимка.
	key *reviewKey
	// edit — один отдельный кандидат правки catalog; nil вне подтверждения правки.
	edit *catalogEdit
	// target — проверенная цель, включая наблюдение существования.
	target *targetState
	// saving — показанный план записи и его окончательное согласие.
	saving *saveReview
	// result — фактическое состояние после safewrite, в том числе при ошибке.
	result *safewrite.Result
	// finished — записанная либо отменённая операция больше не изменяется.
	finished bool
}

// NewCreate сохраняет параметры, разрешая sourceDirectory через inputPath.
// Создаёт собственный reviewKey; нулевые и скопированные Operation не принимают
// токены исходной операции. Остальные конструкторы соблюдают тот же контракт.
// Названия проверяет catalog при анализе; файловая запись отсутствует.
// baseDir — явный полный каталог из main, не настройка через флаги/окружение.
func NewCreate(baseDir string, sourceDirectory string, courseTitle string, unitTitle string) (*Operation, []*diagnostic.Error) {
	base, issue := filetree.Resolve("", baseDir)
	if issue != nil {
		return nil, []*diagnostic.Error{issue}
	}
	source, issue := inputPath(base, sourceDirectory)
	if issue != nil {
		return nil, []*diagnostic.Error{issue}
	}
	op := &Operation{mode: Create, baseDir: base, sourcePath: source, courseTitle: courseTitle, unitTitle: unitTitle}
	op.key = &reviewKey{owner: op}
	return op, nil
}

// NewUpdate сохраняет пути архива и ровно одного JSON; чтение выполняет Analyze.
func NewUpdate(baseDir string, archivePath string, jsonPath string) (*Operation, []*diagnostic.Error) {
	base, issue := filetree.Resolve("", baseDir)
	if issue != nil {
		return nil, []*diagnostic.Error{issue}
	}
	archive, issue := inputPath(base, archivePath)
	if issue != nil {
		return nil, []*diagnostic.Error{issue}
	}
	source, issue := inputPath(base, jsonPath)
	if issue != nil {
		return nil, []*diagnostic.Error{issue}
	}
	op := &Operation{mode: Update, baseDir: base, archivePath: archive, sourcePath: source}
	op.key = &reviewKey{owner: op}
	return op, nil
}

// NewAdd сохраняет пути существующего архива и JSON либо каталога новых уроков.
func NewAdd(baseDir string, archivePath string, sourcePath string) (*Operation, []*diagnostic.Error) {
	base, issue := filetree.Resolve("", baseDir)
	if issue != nil {
		return nil, []*diagnostic.Error{issue}
	}
	archive, issue := inputPath(base, archivePath)
	if issue != nil {
		return nil, []*diagnostic.Error{issue}
	}
	source, issue := inputPath(base, sourcePath)
	if issue != nil {
		return nil, []*diagnostic.Error{issue}
	}
	op := &Operation{mode: Add, baseDir: base, archivePath: archive, sourcePath: source}
	op.key = &reviewKey{owner: op}
	return op, nil
}

// Cancel отбрасывает неподтверждённую операцию и делает все токены недействительными.
// Уже установленный результат не откатывает и служебные файлы не удаляет.
// Идемпотентен, nil получателя ничего не меняет; console вызывает также при EOF
// и перед выходом из программы. После возврата console показывает главное меню.
func (op *Operation) Cancel() {
	if op == nil {
		return
	}
	op.key = nil
	op.edit = nil
	op.saving = nil
	op.finished = true
}

// Retry очищает решения, правки, цель создания, подтверждения и результаты
// прежнего неуспешного анализа; создаёт новый Resolver/Registry и вызывает Analyze.
// Использует первоначальные параметры; для новых параметров console начинает
// новую операцию. После Installed или сохранённых recovery-файлов недоступен:
// сначала пользователь получает результат и начинает новую операцию осознанно.
func (op *Operation) Retry(at time.Time) []*diagnostic.Error {
	if op == nil || op.key == nil || op.key.owner != op {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Операция не создана либо не владеет своим состоянием."}}
	}
	if op.result != nil {
		if op.result.Installed {
			return []*diagnostic.Error{{Code: CodeState, Reason: "Результат уже установлен; начните новую операцию.", Context: diagnostic.Context{Path: op.result.TargetPath}}}
		}
		if op.result.TemporaryPath != "" || op.result.OriginalPath != "" && op.result.OriginalPath != op.result.TargetPath {
			return []*diagnostic.Error{{
				Code: CodeRecovery, Reason: "Перед новой операцией необходимо рассмотреть сохранённые файлы восстановления.",
				Context: diagnostic.Context{Path: op.result.TargetPath},
				Details: []diagnostic.Detail{
					{Label: "Сохранённый исходник", Value: op.result.OriginalPath},
					{Label: "Временный результат", Value: op.result.TemporaryPath},
				},
			}}
		}
	}
	if op.finished || op.tree == nil || len(op.issues) == 0 {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Повторный анализ доступен только после ошибки анализа или блокировки текущего плана."}}
	}
	*op = Operation{
		mode: op.mode, baseDir: op.baseDir, sourcePath: op.sourcePath,
		archivePath: op.archivePath, courseTitle: op.courseTitle, unitTitle: op.unitTitle,
	}
	op.key = &reviewKey{owner: op}
	return op.Analyze(at)
}
