package resourcefind

import (
	"slices"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

const (
	// CodeNotFound — нет кандидата обязательного медиа в автоматической области.
	CodeNotFound = "resourcefind.not_found"

	// CodeAmbiguous — несколько различных кандидатов основного или сопутствующего файла.
	CodeAmbiguous = "resourcefind.ambiguous"

	// CodeExtension — расширение явного основного файла не подходит объявленному типу.
	CodeExtension = "resourcefind.extension"

	// CodeKind — явный основной ресурс не является обычным файлом.
	CodeKind = "resourcefind.kind"
)

// contextualize дополняет существующую ошибку filetree контекстом обращения.
// Сохраняет исходные Code, Reason, Context, Related, Details и Cause;
// добавляет контекст JSON с известными ID и координатами в Related.
// Не заменяет конкретный ошибочный путь путём JSON и не меняет исходную ошибку.
// Возвращает независимую диагностику с независимыми срезами.
func contextualize(err *diagnostic.Error, context diagnostic.Context) *diagnostic.Error {
	if err == nil {
		return nil
	}
	result := *err
	result.Related = slices.Clone(err.Related)
	result.Details = slices.Clone(err.Details)
	if context != (diagnostic.Context{}) {
		result.Related = append(result.Related, context)
	}
	return &result
}

// selectionError оформляет предметную ошибку выбора через diagnostic.Error:
// Code и русская Reason заданы владельцем проверки, Context равен context,
// Related содержит все candidatePaths в полном детерминированном порядке.
// Details хранит kind, точное source и каждый каталог directories с русскими
// подписями. kind обозначает audio, video, image, script или subtitles;
// source — явный путь, точная подстрока или ожидаемое имя сопутствующего файла.
// Для ошибки расширения или вида candidatePaths содержит проверенный путь.
// Неизвестные сведения не выдумываются; функция не выводит сообщения.
func selectionError(code string, reason string, context diagnostic.Context, kind string, source string, directories []string, candidatePaths []string) *diagnostic.Error {
	err := &diagnostic.Error{
		Code:    code,
		Reason:  reason,
		Context: context,
		Details: []diagnostic.Detail{
			{Label: "Тип ресурса", Value: kind},
			{Label: "Источник", Value: source},
		},
	}
	for _, directory := range directories {
		err.Details = append(err.Details, diagnostic.Detail{Label: "Каталог поиска", Value: directory})
	}
	paths := slices.Clone(candidatePaths)
	slices.SortStableFunc(paths, filetree.ComparePaths)
	for _, path := range paths {
		err.Related = append(err.Related, diagnostic.Context{Path: path})
	}
	return err
}
