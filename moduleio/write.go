package moduleio

import (
	"archive/zip"
	"io"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

// Write записывает один полный модуль в новый пустой поток кандидата.
// До первой записи проверяет dst на nil и вызывает module.Validate;
// ошибки модели возвращает без записи. context задаёт известный путь операции,
// не используется для открытия файлов. app не меняет value во время вызова.
// Координирует zip.NewWriter, writeContents и обязательный zip.Writer.Close.
// Ошибку закрытия ZIP добавляет после основной ошибки; ни одну не скрывает.
// Успех означает завершённый ZIP, но ещё не Sync, проверку или замену цели.
// Сам dst не закрывает, не синхронизирует и не сохраняет для будущей записи.
// При сбое поток может содержать частичный ZIP: весь кандидат отклоняется,
// откат и удаление временного файла принадлежат внешней транзакции.
func Write(dst io.Writer, value *module.Module, context diagnostic.Context) []*diagnostic.Error {
	if dst == nil {
		return []*diagnostic.Error{{
			Code: CodeWriter, Reason: "Не задан поток для записи модуля.", Context: context,
		}}
	}
	if problems := module.Validate(value, context); len(problems) != 0 {
		return problems
	}
	writer := zip.NewWriter(dst)
	problems := writeContents(writer, value, context)
	if err := writer.Close(); err != nil {
		problems = append(problems, &diagnostic.Error{
			Code: CodeClose, Reason: "Не удалось завершить и закрыть ZIP модуля.",
			Context: context, Cause: err,
		})
	}
	return problems
}

// writeContents записывает manifest.json и затем Files в порядке среза через
// writeManifest и writeLesson; после ошибки прекращает дальнейшие записи.
// Порядок отображения остаётся внутри манифеста и не определяется порядком ZIP.
// Предусловие — успешный module.Validate. Writer закрывает только Write.
func writeContents(writer *zip.Writer, value *module.Module, context diagnostic.Context) []*diagnostic.Error {
	if problems := writeManifest(writer, value.Manifest, context); len(problems) != 0 {
		return problems
	}
	for _, file := range value.Files {
		if problems := writeLesson(writer, file, context); len(problems) != 0 {
			return problems
		}
	}
	return nil
}

// writeManifest получает JSON через manifestBytes и передаёт writeEntry
// единственное имя module.ManifestFileName; не обновляет CreatedAt.
func writeManifest(writer *zip.Writer, value manifest.Manifest, context diagnostic.Context) []*diagnostic.Error {
	context.FileName = module.ManifestFileName
	raw, problem := manifestBytes(value, context)
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	if problem := writeEntry(writer, module.ManifestFileName, raw, context); problem != nil {
		return []*diagnostic.Error{problem}
	}
	return nil
}

// writeLesson получает точные байты либо новую сериализацию через lessonBytes,
// затем передаёт writeEntry имя File.FileName без переименования.
// Для ошибки подготовки использует File.Context; для записи — известный путь
// кандидата из context, имя и ID урока, сохраняя источник в Related.
func writeLesson(writer *zip.Writer, value *module.File, context diagnostic.Context) []*diagnostic.Error {
	raw, problem := lessonBytes(value)
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	source := value.Context()
	context.FileName = value.FileName()
	context.LessonID = source.LessonID
	if problem := writeEntry(writer, value.FileName(), raw, context); problem != nil {
		problem.Related = append(problem.Related, source)
		return []*diagnostic.Error{problem}
	}
	return nil
}

// writeEntry создаёт одну обычную запись корня ZIP с методом zip.Deflate
// и записывает все raw. Короткая запись и ошибки сжатия/потока дают CodeWrite.
// Предусловие — проверенное уникальное имя и готовые байты JSON.
// Не создаёт каталоги и не сохраняет заголовки или сжатые байты старого ZIP;
// сохранность урока означает равенство несжатого содержимого записи.
func writeEntry(writer *zip.Writer, name string, raw []byte, context diagnostic.Context) *diagnostic.Error {
	context.FileName = name
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0644)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return &diagnostic.Error{
			Code: CodeWrite, Reason: "Не удалось создать очередную запись ZIP.",
			Context: context, Cause: err,
		}
	}
	written, err := entry.Write(raw)
	if err == nil && written != len(raw) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return &diagnostic.Error{
			Code: CodeWrite, Reason: "Не удалось полностью записать содержимое записи ZIP.",
			Context: context, Cause: err,
		}
	}
	return nil
}
