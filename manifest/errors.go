package manifest

const (
	// CodeShape — неверный JSON-тип, отсутствующее или лишнее поле, недопустимый null.
	CodeShape = "manifest.shape"

	// CodeVersion — formatVersion не соответствует поддерживаемому формату.
	CodeVersion = "manifest.format_version"

	// CodeTitle — название нарушает ограничения соответствующего поля схемы.
	CodeTitle = "manifest.title"

	// CodeCreatedAt — нарушена форма либо календарная допустимость даты UTC.
	CodeCreatedAt = "manifest.created_at"

	// CodeInteger — число строк или ревизия не соответствует integer/minimum.
	CodeInteger = "manifest.integer"

	// CodeContents — пустой курс/юнит, пустая секция или недопустимая форма course.
	CodeContents = "manifest.contents"

	// CodeNullOrder — null order присутствует в массиве из нескольких уроков.
	CodeNullOrder = "manifest.null_order"

	// CodeUniqueItems — один массив содержит полностью одинаковые объекты.
	CodeUniqueItems = "manifest.unique_items"
)
