package compare

// Reason — содержательная причина выбранного уровня замены для preview.
// Это не diagnostic.Error: данные могут быть корректны, но требуют замены.
type Reason struct {
	// Code — один из кодов причины, описанных ниже.
	Code ReasonCode

	// Description — конкретная русская причина без JSON ноды и Base64,
	// например прежнее и новое число детей либо позиции перемещённых страниц.
	Description string

	// OldPointer — адрес массива страниц, массива нод либо страницы
	// в старом снимке по RFC 6901; пуст при отсутствии соответствующего места.
	OldPointer string

	// NewPointer — адрес в кандидате; пуст при отсутствии соответствующего места.
	NewPointer string
}

// ReasonCode — устойчивое обозначение причины уровня замены.
type ReasonCode string

const (
	// PageCountChanged — добавлены или удалены страницы, их количество различно.
	PageCountChanged ReasonCode = "page_count_changed"
	// PageOrderChanged — содержимое устанавливает изменение последовательности.
	PageOrderChanged ReasonCode = "page_order_changed"
	// PageCorrespondenceAmbiguous — наблюдается межстраничный сдвиг,
	// но однозначное соответствие изменённого состава установить невозможно.
	PageCorrespondenceAmbiguous ReasonCode = "page_correspondence_ambiguous"
	// NodeCountChanged — различно число корневых нод страницы.
	NodeCountChanged ReasonCode = "node_count_changed"
	// NodeNestingChanged — различно число детей на соответствующей позиции,
	// в том числе при прежнем общем числе нод: изменилась вложенность.
	NodeNestingChanged ReasonCode = "node_nesting_changed"
)
