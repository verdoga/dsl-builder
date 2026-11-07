package catalog

const (
	// CodeState — отсутствует рабочий снимок или нарушен внутренний реестр секций.
	CodeState = "catalog.state"
	// CodeEmpty — пустой набор уроков текущего создаваемого/добавляемого состава.
	CodeEmpty = "catalog.empty"
	// CodeMixedSections — исходный или итоговый состав смешивает размещения.
	CodeMixedSections = "catalog.mixed_sections"
	// CodeUnassigned — распределение всех уроков по секциям не завершено.
	CodeUnassigned = "catalog.unassigned"
	// CodeDuplicateSection — несколько объектов секций с одним точным именем.
	CodeDuplicateSection = "catalog.duplicate_section"
	// CodePartialWeight — исходный вес задан лишь части группы новой сборки.
	CodePartialWeight = "catalog.partial_weight"
	// CodeMissingWeight — итоговый вес не задан, требуется явный ввод.
	CodeMissingWeight = "catalog.missing_weight"
	// CodeWeightConflict — численно совпавшие веса уроков одной группы.
	CodeWeightConflict = "catalog.weight_conflict"
	// CodeLessonOrder — последовательность уроков не соответствует весам.
	CodeLessonOrder = "catalog.lesson_order"
	// CodeSectionWeights — неполный/повторный набор имён или отсутствующий вес секции.
	CodeSectionWeights = "catalog.section_weights"
	// CodeSelection — техническое имя не выбирает ровно одну запись.
	CodeSelection = "catalog.selection"
	// CodeIdentity — синхронизация пытается изменить стабильный ID записи.
	CodeIdentity = "catalog.identity"
)
