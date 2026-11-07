package revision

import (
	"encoding/json"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

// validateUpdateInputs координирует lesson.Validate обоих снимков,
// проверку равенства identity.LessonKey их ID и ValidateHistory для old.
// Кандидат прошёл Initial; его служебные ID и ревизия не переносятся.
// Несовпадение ключей даёт CodeLessonMismatch. Ошибки старого снимка
// предшествуют ошибкам кандидата; неисправный вход не порождает каскад
// арифметических ошибок и не допускает применения даже пустого плана.
func validateUpdateInputs(old *lesson.Lesson, candidate *lesson.Lesson, oldContext diagnostic.Context, candidateContext diagnostic.Context) []*diagnostic.Error {
	problems := lesson.Validate(old, oldContext)
	if len(problems) == 0 {
		problems = append(problems, ValidateHistory(old, oldContext)...)
	}
	candidateProblems := lesson.Validate(candidate, candidateContext)
	problems = append(problems, candidateProblems...)
	if len(problems) != 0 {
		return problems
	}
	if identity.LessonKey(old.Document.ID) != identity.LessonKey(candidate.Document.ID) {
		oldContext.LessonID, candidateContext.LessonID = old.Document.ID, candidate.Document.ID
		problem := revisionError(CodeLessonMismatch, "Старый урок и кандидат имеют разные ID", "/document/id", oldContext)
		problem.Related = []diagnostic.Context{oldContext, candidateContext}
		return []*diagnostic.Error{problem}
	}
	return problems
}

// validatePlan проверяет непустой указатель plan, validateDecisions,
// validateFieldAddresses и validatePageChanges. Это проверки исполнимости
// неизменённого результата compare.Build; сравнение содержимого и полноту
// найденных отличий повторно не выполняет. Ошибки дают CodePlan.
// ReplaceLesson требует пустой Pages и непустой Reasons; иначе Reasons
// пуст. При отсутствии полной замены число страниц снимков одинаково.
func validatePlan(old *lesson.Lesson, candidate *lesson.Lesson, plan *compare.Plan, decisions []Decision, context diagnostic.Context) []*diagnostic.Error {
	if plan == nil {
		return []*diagnostic.Error{revisionError(CodePlan, "План обновления не задан", "/plan", context)}
	}
	problems := validateDecisions(plan.Fields, decisions, context)
	if old == nil || candidate == nil {
		return append(problems, revisionError(CodePlan, "Снимки для проверки плана не заданы", "/plan", context))
	}
	if plan.ReplaceLesson && (len(plan.Pages) != 0 || len(plan.Reasons) == 0) || !plan.ReplaceLesson && len(plan.Reasons) != 0 {
		problems = append(problems, revisionError(CodePlan, "Полная замена требует причин и исключает изменения отдельных страниц", "/plan", context))
	}
	if !plan.ReplaceLesson && len(old.Document.Pages) != len(candidate.Document.Pages) {
		problems = append(problems, revisionError(CodePlan, "Без полной замены число страниц должно совпадать", "/plan/pages", context))
	}
	problems = append(problems, validateFieldAddresses(old, candidate, plan, context)...)
	return append(problems, validatePageChanges(old, candidate, plan.Pages, context)...)
}

// validateFieldAddresses проверяет известный Field, допустимые индексы,
// отсутствие повторных адресов и корректную форму значений FieldChange.
// Поля документа имеют OldPage=NewPage=-1; у DocumentTitle оба значения
// не nil. PageTitle имеет существующий NewPage; OldPage либо существует,
// либо равен -1 только при полной замене и тогда Before=nil.
// При частичной замене OldPage=NewPage. Устанавливать безопасную пару
// повторно или подбирать Before по позиции функция не вправе.
// Связь Before/After с неизменными снимками гарантирует compare.Build.
func validateFieldAddresses(old *lesson.Lesson, candidate *lesson.Lesson, plan *compare.Plan, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	seen, oldPages := map[string]bool{}, map[int]bool{}
	previousRank, previousPage := -1, -1
	for i, change := range plan.Fields {
		pointer := "/plan/fields/" + strconv.Itoa(i)
		rank, address, valid := -1, string(change.Field), true
		switch change.Field {
		case compare.DocumentTitle, compare.DocumentSubtitle:
			rank = 0
			if change.Field == compare.DocumentSubtitle {
				rank = 1
			}
			valid = change.OldPage == -1 && change.NewPage == -1
			if change.Field == compare.DocumentTitle {
				valid = valid && change.Before != nil && change.After != nil
			}
		case compare.PageTitle:
			rank = 2
			address += "/" + strconv.Itoa(change.NewPage)
			valid = change.NewPage >= 0 && change.NewPage < len(candidate.Document.Pages) && change.OldPage >= -1 && change.OldPage < len(old.Document.Pages)
			if change.OldPage == -1 {
				valid = valid && plan.ReplaceLesson && change.Before == nil
			} else {
				valid = valid && !oldPages[change.OldPage]
				oldPages[change.OldPage] = true
			}
			valid = valid && (plan.ReplaceLesson || change.OldPage == change.NewPage) && change.NewPage > previousPage
			previousPage = change.NewPage
		default:
			valid = false
		}
		if !valid || seen[address] || rank < previousRank {
			problems = append(problems, revisionError(CodePlan, "Некорректное поле, адрес, порядок или форма значений изменения", pointer, context))
		}
		seen[address], previousRank = true, rank
	}
	return problems
}

// validatePageChanges проверяет существование и возрастающий порядок
// уникальных индексов страниц. ReplaceNodes требует Nodes пуст и Reasons
// непуст; точечная замена — наоборот. Пути проверяет validateNodeChanges.
// Не определяет причины структурных изменений вместо compare.
func validatePageChanges(old *lesson.Lesson, candidate *lesson.Lesson, changes []compare.PageChange, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	previous := -1
	for i, change := range changes {
		pointer := "/plan/pages/" + strconv.Itoa(i)
		if change.Index < 0 || change.Index >= len(old.Document.Pages) || change.Index >= len(candidate.Document.Pages) || change.Index <= previous {
			problems = append(problems, revisionError(CodePlan, "Индекс страницы отсутствует, повторён или нарушает порядок", pointer+"/index", context))
			continue
		}
		previous = change.Index
		pageContext := context
		pageContext.PageID = old.Document.Pages[change.Index].ID
		if change.ReplaceNodes && (len(change.Nodes) != 0 || len(change.Reasons) == 0) || !change.ReplaceNodes && (len(change.Nodes) == 0 || len(change.Reasons) != 0) {
			problems = append(problems, revisionError(CodePlan, "Форма замены нод страницы противоречит её уровню", pointer, pageContext))
		}
		nodeProblems := validateNodeChanges(old.Document.Pages[change.Index].Nodes, candidate.Document.Pages[change.Index].Nodes, change.Nodes, pageContext)
		for _, problem := range nodeProblems {
			for j := range problem.Details {
				if problem.Details[j].Label == "Поле JSON" {
					problem.Details[j].Value = pointer + problem.Details[j].Value
				}
			}
		}
		problems = append(problems, nodeProblems...)
	}
	return problems
}

// validateNodeChanges проверяет непустые уникальные Path, существование
// каждой цепочки индексов в обоих деревьях и порядок родитель → дети.
// Fields непуст, содержит только известные compare.NodeField без повторов.
// Родитель и его ребёнок могут оба изменяться; это не конфликт путей.
// Не сравнивает Type/Flags/Data и не выводит изменения из значений ID.
func validateNodeChanges(old []lesson.Node, candidate []lesson.Node, changes []compare.NodeChange, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	var previous []int
	for i, change := range changes {
		pointer := "/nodes/" + strconv.Itoa(i)
		valid := len(change.Path) > 0 && (i == 0 || slices.Compare(previous, change.Path) < 0)
		left, right := old, candidate
		for _, index := range change.Path {
			if index < 0 || index >= len(left) || index >= len(right) {
				valid = false
				break
			}
			left, right = left[index].Nodes, right[index].Nodes
		}
		if !valid {
			problems = append(problems, revisionError(CodePlan, "Путь ноды пуст, отсутствует, повторён или нарушает порядок обхода", pointer+"/path", context))
		}
		previous = change.Path
		previousField := -1
		if len(change.Fields) == 0 {
			problems = append(problems, revisionError(CodePlan, "Не указаны изменённые собственные поля ноды", pointer+"/fields", context))
		}
		for j, field := range change.Fields {
			rank := slices.Index([]compare.NodeField{compare.NodeTypeChanged, compare.NodeFlagsChanged, compare.NodeDataChanged}, field)
			if rank < 0 || rank <= previousField {
				problems = append(problems, revisionError(CodePlan, "Поле ноды неизвестно, повторено или нарушает порядок", pointer+"/fields/"+strconv.Itoa(j), context))
			}
			previousField = rank
		}
	}
	return problems
}

// ValidateHistory проверяет только наблюдаемые инварианты одной версии:
// уникальность базовых номеров нод во всём уроке, отсутствие суффиксов
// в ревизии 1, отсутствие суффикса больше Document.Version.
// Предусловие: lesson.Validate успешно проверил модель и полные ID.
// Ноды без суффикса допустимы и в поздних ревизиях; суффикс меньше текущей
// ревизии допустим. Не требует сплошной нумерации или одинаковых суффиксов,
// не восстанавливает отсутствующую историю прежних версий.
// Предназначена также для module при проверке загруженного урока, если
// её вызовет app: зависимость module от revision для этого не требуется.
// Использует validateHistoryNode в порядке страниц и полного дерева.
func ValidateHistory(value *lesson.Lesson, context diagnostic.Context) []*diagnostic.Error {
	if value == nil {
		return lesson.Validate(value, context)
	}
	context.LessonID = value.Document.ID
	seen := make(map[string]string)
	var problems []*diagnostic.Error
	for i, page := range value.Document.Pages {
		pageContext := context
		pageContext.PageID = page.ID
		for j, node := range page.Nodes {
			pointer := "/document/pages/" + strconv.Itoa(i) + "/nodes/" + strconv.Itoa(j)
			problems = append(problems, validateHistoryNode(node, &value.Document, seen, pointer, pageContext)...)
		}
	}
	return problems
}

// validateHistoryNode разбирает ID через identity.ParseNodeID, передаёт
// части validateNodeRevision и регистрирует Number в seen с JSON Pointer.
// Повтор Number с иным суффиксом даёт CodeHistory с обоими адресами;
// потомки проверяются отдельно с тем же индексом всего урока.
func validateHistoryNode(value lesson.Node, document *lesson.Document, seen map[string]string, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	parts, problem := identity.ParseNodeID(value.ID)
	if problem != nil {
		problem.Context = context
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: pointer + "/id"})
		problems = append(problems, problem)
	} else {
		if problem := validateNodeRevision(parts, document, pointer, context); problem != nil {
			problems = append(problems, problem)
		}
		if first, exists := seen[parts.Number]; exists {
			problem := revisionError(CodeHistory, "Базовый номер ноды повторяется в уроке", pointer+"/id", context)
			firstContext := context
			pageIndex, _ := strconv.Atoi(strings.Split(first, "/")[3])
			firstContext.PageID = document.Pages[pageIndex].ID
			problem.Related = []diagnostic.Context{firstContext, context}
			problem.Details = append(problem.Details, diagnostic.Detail{Label: "Первое поле JSON", Value: first}, diagnostic.Detail{Label: "ID ноды", Value: value.ID})
			problems = append(problems, problem)
		} else {
			seen[parts.Number] = pointer + "/id"
		}
	}
	for i, node := range value.Nodes {
		problems = append(problems, validateHistoryNode(node, document, seen, pointer+"/nodes/"+strconv.Itoa(i), context)...)
	}
	return problems
}

// validateNodeRevision сравнивает parts.Revision с математическим значением
// document.Version через integerValue, без машинного ограничения чисел.
// Ошибки содержат ID и адрес ноды; содержимое ноды не выводится.
func validateNodeRevision(parts identity.NodeIDParts, document *lesson.Document, pointer string, context diagnostic.Context) *diagnostic.Error {
	if parts.Revision == "" {
		return nil
	}
	version := integerValue(document.Version)
	revision := integerValue(json.Number(parts.Revision))
	reason := ""
	if version.Cmp(big.NewInt(1)) == 0 {
		reason = "В первой ревизии суффикс версии ноды недопустим"
	} else if revision.Cmp(version) > 0 {
		reason = "Суффикс версии ноды превышает ревизию урока"
	}
	if reason == "" {
		return nil
	}
	problem := revisionError(CodeHistory, reason, pointer+"/id", context)
	id, _ := identity.FormatNodeID(parts.Prefix, parts.Number, parts.Revision)
	problem.Details = append(problem.Details, diagnostic.Detail{Label: "ID ноды", Value: id})
	return problem
}
