package parsed

import (
	"encoding/json"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// decodeDocument читает обязательные поля документа, сохраняя неизвестные
// сведения как null. Метаданные не восстанавливаются из строк или имени JSON.
func decodeDocument(raw json.RawMessage, field string, context diagnostic.Context) (model.Document, *diagnostic.Error) {
	fields, err := decodeObject(raw, []string{
		"dslVersion", "fileName", "filePath", "encoding", "hasBom",
		"lineCount", "byteLength", "sha256", "metadata", "hasErrors",
	}, field, context)
	if err != nil {
		return model.Document{}, err
	}
	var document model.Document
	if err := decodeScalar(fields["dslVersion"], &document.DSLVersion, true, field+".dslVersion", context); err != nil {
		return model.Document{}, err
	}
	if err := decodeScalar(fields["fileName"], &document.FileName, true, field+".fileName", context); err != nil {
		return model.Document{}, err
	}
	if err := decodeScalar(fields["filePath"], &document.FilePath, true, field+".filePath", context); err != nil {
		return model.Document{}, err
	}
	if err := decodeScalar(fields["encoding"], &document.Encoding, true, field+".encoding", context); err != nil {
		return model.Document{}, err
	}
	if err := decodeScalar(fields["hasBom"], &document.HasBOM, true, field+".hasBom", context); err != nil {
		return model.Document{}, err
	}
	if err := decodeScalar(fields["lineCount"], &document.LineCount, true, field+".lineCount", context); err != nil {
		return model.Document{}, err
	}
	if err := decodeScalar(fields["byteLength"], &document.ByteLength, true, field+".byteLength", context); err != nil {
		return model.Document{}, err
	}
	if err := decodeScalar(fields["sha256"], &document.SHA256, true, field+".sha256", context); err != nil {
		return model.Document{}, err
	}
	document.Metadata, err = decodeMetadata(fields["metadata"], field+".metadata", context)
	if err != nil {
		return model.Document{}, err
	}
	if document.Metadata.DocumentID != nil {
		context.LessonID = *document.Metadata.DocumentID
	}
	if err := decodeScalar(fields["hasErrors"], &document.HasErrors, false, field+".hasErrors", context); err != nil {
		return model.Document{}, err
	}
	return document, nil
}

// decodeMetadata читает обязательные nullable-метаданные без подстановок.
// Отсутствующий documentId даёт CodeMissingID; явный null сохраняется.
func decodeMetadata(raw json.RawMessage, field string, context diagnostic.Context) (model.DocumentMetadata, *diagnostic.Error) {
	fields, err := decodeObject(raw, nil, field, context)
	if err != nil {
		return model.DocumentMetadata{}, err
	}
	if _, present := fields["documentId"]; !present {
		return model.DocumentMetadata{}, inputError(CodeMissingID, "Отсутствует обязательный ID урока.", field+".documentId", context, nil)
	}
	var metadata model.DocumentMetadata
	if err := decodeScalar(fields["documentId"], &metadata.DocumentID, true, field+".documentId", context); err != nil {
		return model.DocumentMetadata{}, err
	}
	if metadata.DocumentID != nil {
		context.LessonID = *metadata.DocumentID
	}
	if err := decodeScalar(fields["title"], &metadata.Title, true, field+".title", context); err != nil {
		return model.DocumentMetadata{}, err
	}
	if err := decodeScalar(fields["subtitle"], &metadata.Subtitle, true, field+".subtitle", context); err != nil {
		return model.DocumentMetadata{}, err
	}
	if err := decodeScalar(fields["section"], &metadata.Section, true, field+".section", context); err != nil {
		return model.DocumentMetadata{}, err
	}
	if err := decodeScalar(fields["order"], &metadata.Order, true, field+".order", context); err != nil {
		return model.DocumentMetadata{}, err
	}
	metadata.ResourceDirs, err = decodeStrings(fields["resourceDirs"], true, field+".resourceDirs", context)
	if err != nil {
		return model.DocumentMetadata{}, err
	}
	return metadata, nil
}

// decodeProcessing читает пять обязательных полей ненулевой записи запуска.
// id/tool не допускают null; version/startedAt/durationMs сохраняют null.
func decodeProcessing(raw json.RawMessage, field string, context diagnostic.Context) (model.Processing, *diagnostic.Error) {
	fields, err := decodeObject(raw, []string{
		"id", "tool", "version", "startedAt", "durationMs",
	}, field, context)
	if err != nil {
		return model.Processing{}, err
	}
	var processing model.Processing
	if err := decodeScalar(fields["id"], &processing.ID, false, field+".id", context); err != nil {
		return model.Processing{}, err
	}
	if err := decodeScalar(fields["tool"], &processing.Tool, false, field+".tool", context); err != nil {
		return model.Processing{}, err
	}
	if err := decodeScalar(fields["version"], &processing.Version, true, field+".version", context); err != nil {
		return model.Processing{}, err
	}
	if err := decodeScalar(fields["startedAt"], &processing.StartedAt, true, field+".startedAt", context); err != nil {
		return model.Processing{}, err
	}
	if err := decodeScalar(fields["durationMs"], &processing.DurationMs, true, field+".durationMs", context); err != nil {
		return model.Processing{}, err
	}
	return processing, nil
}
