package filetree

import "github.com/verdoga/dsl-builder/diagnostic"

const (
	// CodePath — путь пуст, некорректен или не разрешается относительно явной базы.
	CodePath = "filetree.path"

	// CodeAccess — ошибка доступа, отсутствующий объект или ошибка чтения каталога.
	CodeAccess = "filetree.access"

	// CodeLink — невозможно разрешить файловую ссылку, в том числе из-за петли.
	CodeLink = "filetree.link"

	// CodeKind — вместо требуемого файла или каталога обнаружен другой вид объекта.
	CodeKind = "filetree.kind"

	// CodeExtension — явно выбранный входной файл имеет расширение не .json.
	CodeExtension = "filetree.extension"

	// CodeEmptyJSON — в выбранном каталоге не найдено ни одного JSON-файла.
	CodeEmptyJSON = "filetree.empty_json"
)

// pathError создаёт diagnostic.Error с русской Reason, кодом владельца
// проверки, конкретным путём в Context.Path и исходной ошибкой в Cause.
// Если полного пути ещё нет, сохраняет исходное обращение; ничего не выводит.
// Вызывающий профильный пакет может дополнить Related и Details данными урока,
// исходной строкой, видом ресурса и областью поиска, не теряя ошибочный путь.
func pathError(code string, reason string, path string, cause error) *diagnostic.Error {
	return &diagnostic.Error{
		Code:    code,
		Reason:  reason,
		Context: diagnostic.Context{Path: path},
		Cause:   cause,
	}
}
