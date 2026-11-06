package model

// Code — устойчивый машинный код диагностики.
type Code string

// Severity — уровень серьёзности диагностики по JSON-контракту.
type Severity string

const (
	SeverityError          Severity = "error"          // Ошибка.
	SeverityWarning        Severity = "warning"        // Предупреждение.
	SeverityRecommendation Severity = "recommendation" // Рекомендация.
)

// Scope — область диагностики в JSON-контракте.
type Scope string

const (
	ScopeElement  Scope = "element"  // Диагностика элемента строки.
	ScopeLine     Scope = "line"     // Диагностика строки.
	ScopeBlock    Scope = "block"    // Диагностика блочной конструкции.
	ScopeDocument Scope = "document" // Диагностика документа.
)
