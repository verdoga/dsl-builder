package moduleio

import (
	"encoding/json"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

// lessonBytes возвращает File.OriginalBytes дословно при ненулевом результате.
// Не меняет пробелы, порядок ключей, неизвестные допустимые свойства и перевод
// строки в конце; не выполняет повторную сериализацию неизменённого урока.
// Для File, созданного NewFile/ReplaceFile с nil OriginalBytes, сериализует
// File.Lesson стандартным encoding/json.MarshalIndent с двумя пробелами,
// без BOM; ошибка сериализации даёт CodeSerialize и nil байты.
// Предусловие — успешный module.Validate; доверие к связи raw и модели
// обеспечивается неизменяемым module.File, повторной проверки связи здесь нет.
func lessonBytes(value *module.File) ([]byte, *diagnostic.Error) {
	if raw := value.OriginalBytes(); raw != nil {
		return raw, nil
	}
	raw, err := json.MarshalIndent(value.Lesson(), "", "  ")
	if err != nil {
		return nil, &diagnostic.Error{
			Code: CodeSerialize, Reason: "Не удалось сериализовать урок в JSON.",
			Context: value.Context(), Cause: err,
		}
	}
	return raw, nil
}

// manifestBytes сериализует готовый манифест через encoding/json.MarshalIndent
// с двумя пробелами, UTF-8 без BOM; сохраняет числа, строки и порядок массивов.
// Не использует время системы, не нормализует веса и названия.
// Предусловие — успешный module.Validate. Ошибка даёт nil байты и CodeSerialize
// с именем module.ManifestFileName; собственная JSON-модель не вводится.
func manifestBytes(value manifest.Manifest, context diagnostic.Context) ([]byte, *diagnostic.Error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		context.FileName = module.ManifestFileName
		return nil, &diagnostic.Error{
			Code: CodeSerialize, Reason: "Не удалось сериализовать манифест в JSON.",
			Context: context, Cause: err,
		}
	}
	return raw, nil
}
