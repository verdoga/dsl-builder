package module

import (
	"math/big"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

// AssignFileNames выдаёт count новых имён в порядке входного списка app.
// Для создания existing пуст: имена l001.json и далее назначаются до convert.
// Для добавления existing содержит все файлы проверенного архива; нумерация
// продолжается после максимума, пропуски не занимаются. app сопоставляет
// результат по индексу со входами, уже отсортированными filetree.SortRelative.
// Пакет не сортирует пути повторно, не сортирует уроки и не меняет старые имена.
// count — длина реально размещённого в памяти списка, не диапазон номеров.
// count <= 0 даёт CodeEmpty. Координирует validateFileNames, maxFileNumber
// и allocateNames; при любой ошибке результат nil, частичного назначения нет.
func AssignFileNames(existing []*File, count int, context diagnostic.Context) ([]string, []*diagnostic.Error) {
	if count <= 0 {
		return nil, []*diagnostic.Error{{
			Code: CodeEmpty, Reason: "Количество новых уроков должно быть положительным.", Context: context,
		}}
	}
	if problems := validateFileNames(existing, context); len(problems) != 0 {
		return nil, problems
	}
	names, problem := allocateNames(maxFileNumber(existing), count, context)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	return names, nil
}

// maxFileNumber выбирает максимум среди номеров identity.ParseFileName;
// для пустого списка возвращает строку 0. Предусловие: validateFileNames успешна.
// Сравнивает точные числа средствами math/big, без int/uint64/float64.
func maxFileNumber(files []*File) string {
	var maximum, number big.Int
	for _, file := range files {
		digits, _ := identity.ParseFileName(file.name)
		number.SetString(digits, 10)
		if number.Cmp(&maximum) > 0 {
			maximum.Set(&number)
		}
	}
	return maximum.String()
}

// allocateNames возвращает count последовательных имён после maximum:
// арифметика math/big, форматирование только identity.FormatFileName.
// maximum — неотрицательное десятичное число от maxFileNumber, count > 0.
// После 999/9999 ширина растёт свободно; искусственной верхней границы нет.
func allocateNames(maximum string, count int, context diagnostic.Context) ([]string, *diagnostic.Error) {
	var number big.Int
	number.SetString(maximum, 10)
	one := big.NewInt(1)
	names := make([]string, count)
	for index := range names {
		number.Add(&number, one)
		name, problem := identity.FormatFileName(number.String())
		if problem != nil {
			problem.Context = context
			return nil, problem
		}
		names[index] = name
	}
	return names, nil
}

// validateFileNames проверяет ненулевые файлы, синтаксис имён существующей
// identity.ParseFileName и точную уникальность полных имён. Пустой список
// разрешён для нового модуля. Повтор даёт CodeDuplicateFile со всеми местами.
// Недопустимое имя не исправляется; ошибки identity сохраняют свой код.
func validateFileNames(files []*File, context diagnostic.Context) []*diagnostic.Error {
	contexts := make([]diagnostic.Context, len(files))
	groups := make(map[string][]diagnostic.Context, len(files))
	for index, file := range files {
		if file == nil {
			continue
		}
		source := file.Context()
		if source.Path == "" {
			source.Path = context.Path
		}
		source.FileName = file.name
		contexts[index] = source
		groups[file.name] = append(groups[file.name], source)
	}
	seen := make(map[string]bool, len(groups))
	var problems []*diagnostic.Error
	for index, file := range files {
		if file == nil {
			problems = append(problems, &diagnostic.Error{
				Code: CodeFile, Reason: "Отсутствует файл урока.", Context: context,
			})
			continue
		}
		source := contexts[index]
		if _, problem := identity.ParseFileName(file.name); problem != nil {
			problem.Context = source
			problems = append(problems, problem)
		}
		if seen[file.name] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeDuplicateFile, Reason: "Несколько файлов имеют одинаковое техническое имя.",
				Context: source, Related: append([]diagnostic.Context(nil), groups[file.name]...),
				Details: []diagnostic.Detail{{Label: "Техническое имя", Value: file.name}},
			})
		}
		seen[file.name] = true
	}
	return problems
}
