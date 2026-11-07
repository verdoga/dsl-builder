package moduleio

import (
	"archive/zip"
	"errors"
	"os"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

// Read загружает существующий ZIP по подготовленному пути path только для чтения.
// Координирует zip.OpenReader, readArchive и закрытие zip.ReadCloser.
// Ошибка открытия либо структуры ZIP даёт CodeOpen либо CodeArchive;
// ненулевая ошибка открытия запрещает использовать даже ненулевой reader.
// Созданный reader закрывается при любом результате; ошибка закрытия добавляется
// после основных ошибок и также запрещает возвращать модель.
// При успехе файлы содержат независимые снимки и точные несжатые байты JSON.
// При любой ошибке результат nil, частично прочитанного модуля нет.
// Path в диагностике — path; имена записей передаются отдельно в FileName.
func Read(path string) (*module.Module, []*diagnostic.Error) {
	context := diagnostic.Context{Path: path}
	reader, err := zip.OpenReader(path)
	var value *module.Module
	var problems []*diagnostic.Error
	if err != nil {
		code, reason := CodeArchive, "Не удалось прочитать структуру ZIP-архива."
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			code, reason = CodeOpen, "Не удалось открыть файл архива для чтения."
		}
		problems = append(problems, &diagnostic.Error{
			Code: code, Reason: reason, Context: context, Cause: err,
		})
	} else {
		value, problems = readArchive(&reader.Reader, context)
	}
	if reader != nil {
		if err := reader.Close(); err != nil {
			problems = append(problems, &diagnostic.Error{
				Code: CodeClose, Reason: "Не удалось закрыть архив после чтения.",
				Context: context, Cause: err,
			})
		}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return value, nil
}

// readArchive координирует validateEntries, readContents и module.Validate.
// Следующий этап запускается только после успеха предыдущего; любая ошибка
// даёт nil. Схемы, веса, ID и связи не проверяются повторными местными правилами.
// Все записи успешного архива полностью прочитаны с контролем CRC до возврата.
// Reader принадлежит Read и здесь не закрывается.
func readArchive(reader *zip.Reader, context diagnostic.Context) (*module.Module, []*diagnostic.Error) {
	if problems := validateEntries(reader.File, context); len(problems) != 0 {
		return nil, problems
	}
	value, problems := readContents(reader.File, context)
	if len(problems) != 0 {
		return nil, problems
	}
	if problems := module.Validate(value, context); len(problems) != 0 {
		return nil, problems
	}
	return value, nil
}

// readContents читает все записи проверенного состава через readManifestEntry
// и readLessonEntry. Files сохраняет порядок записей ZIP, не порядок манифеста.
// Не схлопывает файлы по ID или имени и не сортирует содержимое манифеста.
// Собирает независимые ошибки в порядке записей; при любой ошибке возвращает
// nil. Успешный результат ещё требует module.Validate в readArchive.
func readContents(entries []*zip.File, context diagnostic.Context) (*module.Module, []*diagnostic.Error) {
	value := &module.Module{}
	var problems []*diagnostic.Error
	for _, entry := range entries {
		if entry.Name == module.ManifestFileName {
			manifestValue, entryProblems := readManifestEntry(entry, context)
			problems = append(problems, entryProblems...)
			if len(entryProblems) == 0 {
				value.Manifest = *manifestValue
			}
		} else {
			file, entryProblems := readLessonEntry(entry, context)
			problems = append(problems, entryProblems...)
			if len(entryProblems) == 0 {
				value.Files = append(value.Files, file)
			}
		}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return value, nil
}

// readLessonEntry связывает readEntry, decodeLesson и module.LoadedFile.
// Передаёт LoadedFile ровно те raw, из которых декодирован проверенный урок;
// имя берёт из записи, Context дополняет этим именем и прочитанным ID.
// Не вызывает NewFile вместо LoadedFile: исходные байты должны сохраниться.
// При любой ошибке возвращает nil, не прикрепляет частичные байты.
func readLessonEntry(entry *zip.File, context diagnostic.Context) (*module.File, []*diagnostic.Error) {
	context.FileName = entry.Name
	raw, problems := readEntry(entry, context)
	if len(problems) != 0 {
		return nil, problems
	}
	value, problems := decodeLesson(raw, context)
	if len(problems) != 0 {
		return nil, problems
	}
	context.LessonID = value.Document.ID
	return module.LoadedFile(entry.Name, value, raw, context)
}

// readManifestEntry связывает readEntry и decodeManifest для manifest.json.
// Контекст содержит имя module.ManifestFileName. При ошибке возвращает nil;
// исходные байты манифеста сохранять для последующей записи не требуется.
func readManifestEntry(entry *zip.File, context diagnostic.Context) (*manifest.Manifest, []*diagnostic.Error) {
	context.FileName = module.ManifestFileName
	raw, problems := readEntry(entry, context)
	if len(problems) != 0 {
		return nil, problems
	}
	return decodeManifest(raw, context)
}
