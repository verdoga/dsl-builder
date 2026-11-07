package app

const (
	// CodeState — действие неприменимо к текущему этапу или завершённой операции.
	CodeState = "app.state"
	// CodeStaleReview — передан нулевой, чужой либо устаревший токен решения.
	CodeStaleReview = "app.stale_review"
	// CodeDecision — отсутствует отдельное решение либо неверен индекс поля.
	CodeDecision = "app.decision"
	// CodeIncompleteWeights — последовательный набор весов группы неполон.
	CodeIncompleteWeights = "app.incomplete_weights"
	// CodeArchiveName — введённое имя архива недопустимо в Windows.
	CodeArchiveName = "app.archive_name"
	// CodeDestination — цель или её каталог не подходят для записи.
	CodeDestination = "app.destination"
	// CodeTargetChanged — после чтения или показа целевой файл изменился.
	CodeTargetChanged = "app.target_changed"
	// CodeUnconfirmed — нет актуального окончательного согласия на всю запись.
	CodeUnconfirmed = "app.unconfirmed"
	// CodeOverwrite — не получено отдельное разрешение существующей цели.
	CodeOverwrite = "app.overwrite"
	// CodeRecovery — оставшиеся пути требуют явного рассмотрения результата.
	CodeRecovery = "app.recovery"
)

// Диагностика — существующий diagnostic.Error с русской Reason, известными
// путями/ID, Related/Details и сохранённой Cause. Ошибки владельцев правил
// передаются без замены кодов. Ошибка ввода не портит принятую модель;
// ошибки источников требуют Retry либо нового запуска из главного меню.
// Методы решений при nil Operation возвращают CodeState, без паники;
// Snapshot/Cancel отдельно определяют безопасный результат нулевого получателя.
