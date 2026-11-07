package compare

import (
	"fmt"
	"strconv"

	"github.com/verdoga/dsl-builder/lesson"
)

// compareNodeShape сравнивает число и упорядоченную вложенность нод;
// тип, флаги и data не являются формой дерева. Отличие типа при той же
// форме допускает точечную замену. parentPath пуст у корней страницы.
// Разное число корней даёт NodeCountChanged; разное число потомков —
// NodeNestingChanged с адресом соответствующего nodes и точными количествами.
// В несопоставимом массиве не создаёт фиктивные пары детей; в остальных
// ветвях продолжает проверку. Произвольного предела глубины или числа нет.
func compareNodeShape(old []lesson.Node, candidate []lesson.Node, pageIndex int, parentPath []int) []Reason {
	if len(old) != len(candidate) {
		pointer := "/document/pages/" + strconv.Itoa(pageIndex) + "/nodes"
		for _, index := range parentPath {
			pointer += "/" + strconv.Itoa(index) + "/nodes"
		}
		code := NodeCountChanged
		description := fmt.Sprintf("Изменилось количество корневых нод страницы: было %d, стало %d.", len(old), len(candidate))
		if len(parentPath) > 0 {
			code = NodeNestingChanged
			description = fmt.Sprintf("Изменилось количество дочерних нод: было %d, стало %d.", len(old), len(candidate))
		}
		return []Reason{{Code: code, Description: description, OldPointer: pointer, NewPointer: pointer}}
	}
	var reasons []Reason
	for i := range old {
		path := make([]int, len(parentPath)+1)
		copy(path, parentPath)
		path[len(parentPath)] = i
		reasons = append(reasons, compareNodeShape(old[i].Nodes, candidate[i].Nodes, pageIndex, path)...)
	}
	return reasons
}

// compareNodeChanges обходит равные по форме деревья по соответствующим
// позициям: собственные поля через compareNodeOwnFields, затем дети.
// После отличия продолжает проверку всех остальных нод. Две ноды одинаковой
// формы, обменявшиеся местами, образуют две позиционные замены; сопоставления
// по первичным ID и попытки переноса их базовых номеров между позициями нет.
func compareNodeChanges(old []lesson.Node, candidate []lesson.Node, parentPath []int) []NodeChange {
	var changes []NodeChange
	for i := range old {
		path := make([]int, len(parentPath)+1)
		copy(path, parentPath)
		path[len(parentPath)] = i
		if fields := compareNodeOwnFields(old[i], candidate[i]); len(fields) > 0 {
			changes = append(changes, NodeChange{Path: path, Fields: fields})
		}
		changes = append(changes, compareNodeChanges(old[i].Nodes, candidate[i].Nodes, path)...)
	}
	return changes
}

// compareNodeOwnFields сравнивает только Type, Flags и Data; ID и Nodes
// исключены. Возвращает непустые изменения в порядке type, flags, data.
// Равные собственные поля дают пустой результат, даже если изменены дети.
func compareNodeOwnFields(old lesson.Node, candidate lesson.Node) []NodeField {
	var fields []NodeField
	if old.Type != candidate.Type {
		fields = append(fields, NodeTypeChanged)
	}
	if !equalFlags(old.Flags, candidate.Flags) {
		fields = append(fields, NodeFlagsChanged)
	}
	if !equalDataList(old.Data, candidate.Data) {
		fields = append(fields, NodeDataChanged)
	}
	return fields
}

// equalNodeTrees сравнивает полный упорядоченный лес по форме, собственным
// полям через compareNodeOwnFields и потомкам. Все ID игнорируются.
// Используется только для свидетельств порядка страниц, не для поиска
// «похожих» нод и не для определения новой версии.
func equalNodeTrees(left []lesson.Node, right []lesson.Node) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if len(compareNodeOwnFields(left[i], right[i])) > 0 || !equalNodeTrees(left[i].Nodes, right[i].Nodes) {
			return false
		}
	}
	return true
}

// equalFlags сравнивает наличие и значения Speaking, Stepped и Checkable.
// nil отличается от false; адреса указателей не сравниваются. Флаги
// не пересчитываются из данных, в том числе у multifill.
func equalFlags(left lesson.NodeFlags, right lesson.NodeFlags) bool {
	equalBool := func(a, b *bool) bool {
		if a == nil || b == nil {
			return a == nil && b == nil
		}
		return *a == *b
	}
	return equalBool(left.Speaking, right.Speaking) &&
		equalBool(left.Stepped, right.Stepped) &&
		equalBool(left.Checkable, right.Checkable)
}
