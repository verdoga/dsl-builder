// Package resourcefind выбирает медиа и сопутствующие файлы по ТЗ 9.1–9.4.
// Читает сведения файловой системы, но не байты файлов; не строит урок,
// не встраивает ресурсы и не отмечает их использованными.
// Входные значения уже приняты по Contract 1.0; DSL не проверяется повторно.
package resourcefind

import (
	"path/filepath"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

// Finder хранит область одного технического JSON и общий разрешитель операции.
// Создаётся Prepare; вызовы последовательные. После исправления исходников
// вызывающий код начинает новый анализ с новым общим filetree.Resolver.
type Finder struct {
	// tree — единственный Resolver всей операции, общий для всех уроков,
	// ресурсных каталогов, явных путей и сопутствующих файлов.
	tree *filetree.Resolver

	// baseDir — каталог адреса обнаружения технического JSON, не IdentityPath,
	// не document.filePath, не рабочая папка и не корень всей сборки.
	baseDir string

	// directories — полные адреса resourceDirs в порядке объявлений.
	// Повторные объявления сохранены для проверки доступности каждого пути.
	directories []string
}

// Prepare создаёт область для jsonPath — полного Entry.Path технического JSON.
// tree ненулевой и общий для операции; resourceDirs переданы из метаданных.
// context содержит доступные сведения урока и путь этого технического JSON.
// Координирует разрешение каталогов и сбор области учёта отдельными функциями.
// Проверяет каждый указанный каталог, включая повторный или пересекающийся,
// даже если в документе нет медианод. Отсутствие или недоступность — ошибка.
// nil или пустой resourceDirs означает отсутствие автоматической области.
// Возвращает все учтённые IdentityPath через accountedPaths, без отметки
// использования; при ошибке возвращает nil, nil и диагностику.
// Срезы результата независимы от входа; вызывающий код регистрирует пути.
func Prepare(tree *filetree.Resolver, jsonPath string, resourceDirs []string, context diagnostic.Context) (finder *Finder, accountedPaths []string, err *diagnostic.Error) {
	baseDir := filepath.Dir(jsonPath)
	directories, err := resolveDirectories(baseDir, resourceDirs, context)
	if err != nil {
		return nil, nil, err
	}
	finder = &Finder{tree: tree, baseDir: baseDir, directories: directories}
	accountedPaths, err = finder.inventory(context)
	if err != nil {
		return nil, nil, err
	}
	return finder, accountedPaths, nil
}

// Find выбирает файлы для одной медианоды и не изменяет область Finder.
// mediaType — принятое значение audio, video или image; raw и value — точные
// поля единственного source, переданные вызывающим кодом из готового входа.
// context содержит JSON, ID, страницу и исходные координаты, если известны.
// Координирует выбор основного файла, поиск сопутствующих и формирование
// Selection; предметные шаги выполняют отдельные небольшие функции.
// Для image поиск сопутствующих вообще не вызывается.
// При любой ошибке Selection не используется; частичного успеха нет.
// Выбор не гарантирует последующего чтения: его проверяет resourceembed.
func (f *Finder) Find(mediaType string, raw string, value string, context diagnostic.Context) (Selection, *diagnostic.Error) {
	body, err := f.mainFile(mediaType, raw, value, context)
	if err != nil {
		return Selection{}, err
	}
	var script *File
	var subtitles []File
	if mediaType == "audio" || mediaType == "video" {
		script, subtitles, err = f.companions(body, context)
		if err != nil {
			return Selection{}, err
		}
	}
	return selection(body, script, subtitles), nil
}
