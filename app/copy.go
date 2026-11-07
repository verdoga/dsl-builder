package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
)

// cloneModule копирует контейнер и срез Files; сами File можно разделять:
// все их поля закрыты, Lesson/OriginalBytes уже возвращают независимые копии.
// Manifest копируется глубоко: Course, Unit, указатели массивов и все массивы
// секций/записей, Subtitle/Order. Типы берутся из полей value без нового импорта
// manifest и без второй модели. Не экспортирует ошибочный catalog для копирования.
func cloneModule(value *module.Module) *module.Module {
	if value == nil {
		return nil
	}
	result := *value
	result.Files = slices.Clone(value.Files)
	if value.Manifest.Course.Unit != nil {
		unit := *value.Manifest.Course.Unit
		result.Manifest.Course.Unit = &unit
	}
	for _, inUnit := range []bool{false, true} {
		lessons, sections := value.Manifest.Course.Lessons, value.Manifest.Course.Sections
		if inUnit {
			if value.Manifest.Course.Unit == nil {
				continue
			}
			lessons = &value.Manifest.Course.Unit.Lessons
			sections = &value.Manifest.Course.Unit.Sections
		}
		if lessons != nil {
			copied := slices.Clone(*lessons)
			for index := range copied {
				if copied[index].Order != nil {
					order := *copied[index].Order
					copied[index].Order = &order
				}
				if copied[index].Subtitle != nil {
					subtitle := *copied[index].Subtitle
					copied[index].Subtitle = &subtitle
				}
			}
			if inUnit {
				result.Manifest.Course.Unit.Lessons = copied
			} else {
				result.Manifest.Course.Lessons = &copied
			}
		}
		if sections != nil {
			copied := slices.Clone(*sections)
			for index := range copied {
				copied[index].Lessons = slices.Clone(copied[index].Lessons)
				for entry := range copied[index].Lessons {
					lesson := &copied[index].Lessons[entry]
					if lesson.Order != nil {
						order := *lesson.Order
						lesson.Order = &order
					}
					if lesson.Subtitle != nil {
						subtitle := *lesson.Subtitle
						lesson.Subtitle = &subtitle
					}
				}
			}
			if inUnit {
				result.Manifest.Course.Unit.Sections = copied
			} else {
				result.Manifest.Course.Sections = &copied
			}
		}
	}
	return &result
}

// clonePlan копирует весь compare.Plan, включая Fields.Before/After,
// Reasons, Pages.Reasons, Pages.Nodes.Path и Fields каждой ноды.
// Ни уровень замены, ни порядок, ни несопоставленные поля не изменяются.
func clonePlan(value *compare.Plan) *compare.Plan {
	if value == nil {
		return nil
	}
	result := *value
	result.Fields = slices.Clone(value.Fields)
	for index := range result.Fields {
		field := &result.Fields[index]
		if field.Before != nil {
			before := *field.Before
			field.Before = &before
		}
		if field.After != nil {
			after := *field.After
			field.After = &after
		}
	}
	result.Reasons = slices.Clone(value.Reasons)
	result.Pages = slices.Clone(value.Pages)
	for index := range result.Pages {
		page := &result.Pages[index]
		page.Reasons = slices.Clone(page.Reasons)
		page.Nodes = slices.Clone(page.Nodes)
		for node := range page.Nodes {
			page.Nodes[node].Path = slices.Clone(page.Nodes[node].Path)
			page.Nodes[node].Fields = slices.Clone(page.Nodes[node].Fields)
		}
	}
	return &result
}

// cloneChanges копирует catalog.Change, Section и байты Before/After,
// сохраняя отсутствие, null, точные числа и исходные строки.
func cloneChanges(values []catalog.Change) []catalog.Change {
	result := slices.Clone(values)
	for index := range result {
		change := &result[index]
		if change.Section != nil {
			section := *change.Section
			change.Section = &section
		}
		change.Before = slices.Clone(change.Before)
		change.After = slices.Clone(change.After)
	}
	return result
}

// cloneDiagnostics копирует ошибки, Related/Details и структурированные
// diagnostic причины рекурсивно; для цепочек errors.Join сохраняет каждую
// дочернюю причину независимой. Не выдаёт изменяемые внутренние ошибки наружу.
// Стандартные файловые ошибки также копируются со своей цепочкой Cause.
func cloneDiagnostics(values []*diagnostic.Error) []*diagnostic.Error {
	copied := make(map[*diagnostic.Error]*diagnostic.Error)
	var cloneError func(error) error
	cloneError = func(cause error) error {
		switch source := cause.(type) {
		case nil:
			return nil
		case *diagnostic.Error:
			if source == nil {
				return (*diagnostic.Error)(nil)
			}
			if result, exists := copied[source]; exists {
				return result
			}
			result := *source
			copied[source] = &result
			result.Related = slices.Clone(source.Related)
			result.Details = slices.Clone(source.Details)
			result.Cause = cloneError(source.Cause)
			return &result
		case *os.PathError:
			if source == nil {
				return (*os.PathError)(nil)
			}
			result := *source
			result.Err = cloneError(source.Err)
			return &result
		case *os.LinkError:
			if source == nil {
				return (*os.LinkError)(nil)
			}
			result := *source
			result.Err = cloneError(source.Err)
			return &result
		case *os.SyscallError:
			if source == nil {
				return (*os.SyscallError)(nil)
			}
			result := *source
			result.Err = cloneError(source.Err)
			return &result
		case *json.SyntaxError:
			if source == nil {
				return (*json.SyntaxError)(nil)
			}
			result := *source
			return &result
		case *json.UnmarshalTypeError:
			if source == nil {
				return (*json.UnmarshalTypeError)(nil)
			}
			result := *source
			return &result
		case *json.MarshalerError:
			if source == nil {
				return (*json.MarshalerError)(nil)
			}
			result := *source
			result.Err = cloneError(source.Err)
			return &result
		case *strconv.NumError:
			if source == nil {
				return (*strconv.NumError)(nil)
			}
			result := *source
			result.Err = cloneError(source.Err)
			return &result
		case *time.ParseError:
			if source == nil {
				return (*time.ParseError)(nil)
			}
			result := *source
			return &result
		case interface{ Unwrap() []error }:
			children := source.Unwrap()
			result := make([]error, len(children))
			for index, child := range children {
				result[index] = cloneError(child)
			}
			return errors.Join(result...)
		case interface{ Unwrap() error }:
			if source.Unwrap() == nil {
				return errors.New(cause.Error())
			}
			// Нулевая точность сохраняет текст обёртки и отдельную цепочку %w.
			return fmt.Errorf("%s%.0w", cause.Error(), cloneError(source.Unwrap()))
		default:
			return cause
		}
	}
	result := slices.Clone(values)
	for index, value := range values {
		if value != nil {
			result[index] = cloneError(value).(*diagnostic.Error)
		}
	}
	return result
}

// copyEditPreview возвращает независимый catalog.Clone, cloneChanges и
// cloneDiagnostics; внутренний кандидат для AcceptEdit остаётся закрытым.
func (op *Operation) copyEditPreview() *EditPreview {
	if op == nil || op.edit == nil {
		return nil
	}
	return &EditPreview{
		Token:    ReviewToken{key: op.edit.key},
		Catalog:  catalog.Clone(op.edit.candidate),
		Changes:  cloneChanges(catalog.Changes(op.placement, op.edit.candidate)),
		Blockers: cloneDiagnostics(op.edit.issues),
	}
}
