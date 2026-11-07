package moduleio

import (
	"archive/zip"
	"io"
	"strconv"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/module"
)

// validateEntries проверяет физический список ZIP, сохраняя все повторы.
// Для каждой записи вызывает validateEntry, требует ровно один manifest.json
// и запрещает повтор полного имени любой записи, включая манифест.
// Отсутствие манифеста даёт CodeManifest; повтор — CodeDuplicateEntry.
// Related содержит всех участников повтора, Details — их позиции в ZIP,
// поскольку повторяющиеся Path и FileName сами по себе их не различают.
// Порядок ошибок: записи в порядке ZIP, затем отсутствие манифеста.
// Не проверяет ссылки манифеста и не требует непрерывной нумерации уроков.
func validateEntries(entries []*zip.File, context diagnostic.Context) []*diagnostic.Error {
	positions := make(map[string][]int, len(entries))
	for index, entry := range entries {
		positions[entry.Name] = append(positions[entry.Name], index+1)
	}
	var problems []*diagnostic.Error
	for index, entry := range entries {
		problems = append(problems, validateEntry(entry, context)...)
		matches := positions[entry.Name]
		if len(matches) < 2 || matches[0] != index+1 {
			continue
		}
		source := context
		source.FileName = entry.Name
		problem := &diagnostic.Error{
			Code: CodeDuplicateEntry, Reason: "Имя записи ZIP встречается несколько раз.",
			Context: source,
		}
		for _, position := range matches {
			problem.Related = append(problem.Related, source)
			problem.Details = append(problem.Details, diagnostic.Detail{
				Label: "Позиция записи в ZIP (с 1)", Value: strconv.Itoa(position),
			})
		}
		problems = append(problems, problem)
	}
	if len(positions[module.ManifestFileName]) == 0 {
		context.FileName = module.ManifestFileName
		problems = append(problems, &diagnostic.Error{
			Code: CodeManifest, Reason: "В ZIP отсутствует обязательный manifest.json.",
			Context: context,
		})
	}
	return problems
}

// validateEntry запрещает каталоги и записи не обычных файлов, включая ссылки,
// по имени и файловому режиму ZIP. Все имена обязаны находиться в корне.
// Принимает только точное module.ManifestFileName либо имя, принимаемое
// identity.ParseFileName; не дублирует синтаксис и арифметику технических имён.
// Пути с прямыми/обратными разделителями, абсолютные пути, иное написание
// регистра, посторонние файлы и недопустимые имена не очищаются и не исправляются.
// Для неверного имени сохраняет причину identity в Cause ошибки CodeEntryName.
// Context.FileName содержит точное прочитанное имя, а Path — путь архива.
func validateEntry(entry *zip.File, context diagnostic.Context) []*diagnostic.Error {
	context.FileName = entry.Name
	var problems []*diagnostic.Error
	if strings.HasSuffix(entry.Name, "/") || !entry.Mode().IsRegular() {
		problems = append(problems, &diagnostic.Error{
			Code: CodeEntryType, Reason: "Запись ZIP должна быть обычным файлом, а не каталогом или ссылкой.",
			Context: context,
		})
	}
	if entry.Name != module.ManifestFileName {
		if _, cause := identity.ParseFileName(entry.Name); cause != nil {
			cause.Context = context
			problems = append(problems, &diagnostic.Error{
				Code: CodeEntryName, Reason: "Имя записи ZIP не соответствует плоскому формату модуля.",
				Context: context, Cause: cause,
				Details: append([]diagnostic.Detail(nil), cause.Details...),
			})
		}
	}
	return problems
}

// readEntry открывает поток записи и читает до EOF средствами archive/zip,
// чтобы проверить распаковку, полный размер и CRC, включая ошибку последнего
// чтения. Успешный разбор начального JSON не заменяет дочитывания записи.
// Возвращает точные несжатые байты без нормализации; raw ZIP не копируется.
// Закрывает поток и при ошибке чтения, учитывает обе ошибки в порядке чтение,
// закрытие. Неподдерживаемое сжатие и повреждение также запрещают успех.
// При любой ошибке возвращает nil байты; извлечения на диск не выполняет.
func readEntry(entry *zip.File, context diagnostic.Context) ([]byte, []*diagnostic.Error) {
	context.FileName = entry.Name
	reader, err := entry.Open()
	if err != nil {
		return nil, []*diagnostic.Error{{
			Code: CodeRead, Reason: "Не удалось открыть поток записи ZIP.",
			Context: context, Cause: err,
		}}
	}
	raw, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	var problems []*diagnostic.Error
	if readErr != nil {
		problems = append(problems, &diagnostic.Error{
			Code: CodeRead, Reason: "Не удалось полностью прочитать и проверить запись ZIP.",
			Context: context, Cause: readErr,
		})
	}
	if closeErr != nil {
		problems = append(problems, &diagnostic.Error{
			Code: CodeClose, Reason: "Не удалось закрыть поток записи ZIP.",
			Context: context, Cause: closeErr,
		})
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return raw, nil
}
