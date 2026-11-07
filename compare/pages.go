package compare

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/verdoga/dsl-builder/lesson"
)

// pageMatches — точные соответствия содержимого страниц для проверки порядка.
// Требуются списки всех совпадений: выбор первого скрыл бы неоднозначность.
// Имена страниц и любые ID из равенства содержимого исключаются.
type pageMatches struct {
	// OldForNew — для каждой новой страницы индексы всех старых страниц
	// с тем же деревом нод и собственными данными, по возрастанию индекса.
	OldForNew [][]int

	// NewForOld — обратные списки всех новых совпадений для каждой старой;
	// односторонней уникальности недостаточно для безопасной пары.
	NewForOld [][]int
}

// comparePageSequence координирует findPageMatches и classifyPageSequence.
// Возвращает основания полной замены либо пустой список, а также точные
// совпадения для сравнения полей. При полной замене comparePageTitles берёт
// из них только взаимно однозначные пары; остальные страницы остаются без
// установленного прежнего поля. Ни одна пара при полной замене не разрешает
// сохранить старые ID страниц или нод.
func comparePageSequence(old []lesson.Page, candidate []lesson.Page) (pageMatches, []Reason) {
	matches := findPageMatches(old, candidate)
	return matches, classifyPageSequence(old, candidate, matches)
}

// findPageMatches сравнивает содержимое через equalNodeTrees, включая порядок
// нод и data, без названий страниц и генерируемых ID. Даже пустые и повторные
// страницы сохраняют все совпадения. Хеш не служит доказательством равенства.
func findPageMatches(old []lesson.Page, candidate []lesson.Page) pageMatches {
	matches := pageMatches{
		OldForNew: make([][]int, len(candidate)),
		NewForOld: make([][]int, len(old)),
	}
	for newIndex, newPage := range candidate {
		for oldIndex, oldPage := range old {
			if equalNodeTrees(oldPage.Nodes, newPage.Nodes) {
				matches.OldForNew[newIndex] = append(matches.OldForNew[newIndex], oldIndex)
				matches.NewForOld[oldIndex] = append(matches.NewForOld[oldIndex], newIndex)
			}
		}
	}
	return matches
}

// classifyPageSequence только координирует pageCountReason,
// equalPageOrder, movedPageReasons и ambiguousPageReasons.
// Уровень определяется только по наблюдаемым снимкам.
// Разное число страниц всегда даёт PageCountChanged. При равном числе
// полное попозиционное равенство содержимого сохраняет последовательность,
// даже при дубликатах и изменённых названиях: переименование не перестановка.
// Если содержимое различается и взаимно однозначная точная пара смещена,
// выдаётся PageOrderChanged. Неоднозначные точные внепозиционные совпадения
// у изменённых позиций дают PageCorrespondenceAmbiguous; первый кандидат
// не выбирается. Оба случая требуют полной замены без переноса старых ID.
// При отсутствии таких свидетельств сдвига сохраняется позиционное сравнение:
// несколько исправленных страниц сами по себе не требуют полной замены.
// Перестановку одновременно полностью переписанных страниц по двум снимкам
// доказать нельзя; функция не утверждает восстановление истории правок.
// Приоритет причин: количество, доказанный сдвиг, неоднозначность; адреса
// и описание перечисляют конкретные наблюдаемые позиции детерминированно.
func classifyPageSequence(old []lesson.Page, candidate []lesson.Page, matches pageMatches) []Reason {
	if reason := pageCountReason(old, candidate); reason != nil {
		return []Reason{*reason}
	}
	if equalPageOrder(matches) {
		return nil
	}
	if reasons := movedPageReasons(matches); len(reasons) > 0 {
		return reasons
	}
	return ambiguousPageReasons(matches)
}

// pageCountReason возвращает PageCountChanged с прежним и новым количеством
// страниц, либо nil при равенстве. Не сравнивает названия или содержимое.
func pageCountReason(old []lesson.Page, candidate []lesson.Page) *Reason {
	if len(old) == len(candidate) {
		return nil
	}
	return &Reason{
		Code:        PageCountChanged,
		Description: fmt.Sprintf("Изменилось количество страниц: было %d, стало %d.", len(old), len(candidate)),
		OldPointer:  "/document/pages",
		NewPointer:  "/document/pages",
	}
}

// equalPageOrder проверяет наличие собственного индекса в каждом списке
// точных совпадений при равном числе страниц. true включает пустые массивы;
// дубликаты содержимого не нарушают равенство последовательностей.
func equalPageOrder(matches pageMatches) bool {
	if len(matches.OldForNew) != len(matches.NewForOld) {
		return false
	}
	for index, oldIndices := range matches.OldForNew {
		if !slices.Contains(oldIndices, index) {
			return false
		}
	}
	return true
}

// movedPageReasons возвращает причины для взаимно однозначных точных пар,
// у которых различаются индексы старой и новой страницы, по новым индексам.
// При попозиционном равенстве всех страниц не вызывается.
func movedPageReasons(matches pageMatches) []Reason {
	var reasons []Reason
	for newIndex, oldIndices := range matches.OldForNew {
		if len(oldIndices) != 1 {
			continue
		}
		oldIndex := oldIndices[0]
		if oldIndex == newIndex || len(matches.NewForOld[oldIndex]) != 1 || matches.NewForOld[oldIndex][0] != newIndex {
			continue
		}
		reasons = append(reasons, Reason{
			Code:        PageOrderChanged,
			Description: fmt.Sprintf("Содержимое старой страницы на позиции %d точно соответствует новой странице на позиции %d; взаимно однозначная пара смещена.", oldIndex+1, newIndex+1),
			OldPointer:  "/document/pages/" + strconv.Itoa(oldIndex),
			NewPointer:  "/document/pages/" + strconv.Itoa(newIndex),
		})
	}
	return reasons
}

// ambiguousPageReasons выявляет точные внепозиционные совпадения изменённых
// позиций, которые не образуют взаимно однозначной пары. Возвращает конкретные
// причины с участвующими позициями без выбора одного из повторных кандидатов.
// Вызывается при равном числе страниц, отличающемся порядке содержимого
// и отсутствии доказанного сдвига; отсутствие совпадений не выдаётся за
// доказательство переноса страниц и само по себе не порождает причину.
func ambiguousPageReasons(matches pageMatches) []Reason {
	var reasons []Reason
	for newIndex, oldIndices := range matches.OldForNew {
		newPositionEqual := slices.Contains(oldIndices, newIndex)
		for _, oldIndex := range oldIndices {
			newIndices := matches.NewForOld[oldIndex]
			if oldIndex == newIndex || newPositionEqual && slices.Contains(newIndices, oldIndex) {
				continue
			}
			if len(oldIndices) == 1 && len(newIndices) == 1 && newIndices[0] == newIndex {
				continue
			}
			reasons = append(reasons, Reason{
				Code:        PageCorrespondenceAmbiguous,
				Description: fmt.Sprintf("Содержимое новой страницы на позиции %d точно совпадает со старой страницей на позиции %d вне соответствующей позиции; точных старых совпадений для новой страницы: %d, новых для старой: %d. Однозначное соответствие изменённой позиции не установлено.", newIndex+1, oldIndex+1, len(oldIndices), len(newIndices)),
				OldPointer:  "/document/pages/" + strconv.Itoa(oldIndex),
				NewPointer:  "/document/pages/" + strconv.Itoa(newIndex),
			})
		}
	}
	return reasons
}

// comparePages сравнивает каждую соответствующую страницу через comparePage.
// Вызывается только при отсутствии полной замены; размеры массивов одинаковы.
// Продолжает обход после любого отличия и возвращает только изменённые страницы.
func comparePages(old []lesson.Page, candidate []lesson.Page) []PageChange {
	var changes []PageChange
	for index := range candidate {
		if change := comparePage(old[index], candidate[index], index); change != nil {
			changes = append(changes, *change)
		}
	}
	return changes
}

// comparePage сначала получает compareNodeShape. Непустые причины дают
// ReplaceNodes без попарных NodeChange; иначе compareNodeChanges выявляет
// собственные изменения всех соответствующих нод. nil означает отсутствие
// изменений нод; отличие названия отдельно учитывает comparePageTitles.
func comparePage(old lesson.Page, candidate lesson.Page, index int) *PageChange {
	reasons := compareNodeShape(old.Nodes, candidate.Nodes, index, nil)
	if len(reasons) > 0 {
		return &PageChange{Index: index, ReplaceNodes: true, Reasons: reasons}
	}
	changes := compareNodeChanges(old.Nodes, candidate.Nodes, nil)
	if len(changes) == 0 {
		return nil
	}
	return &PageChange{Index: index, Nodes: changes}
}
