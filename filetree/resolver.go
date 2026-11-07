package filetree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// Resolver закрепляет единый IdentityPath для одного объекта файловой системы.
// Нулевое значение готово к работе; один экземпляр передают всем обработчикам
// уроков, областей ресурсов и явных путей всей текущей операции.
// После изменения не копируют по значению; доступ последовательный.
// Повторный анализ после исправления файлов начинает новый Resolver.
// Состояние не означает, что файл уже включён в результат очередного обхода.
type Resolver struct {
	// identities — ранее встреченные объекты и закреплённые за ними пути;
	// совпадение подтверждается файловой идентичностью через os.SameFile.
	identities []identity
}

// identity связывает стандартные сведения об объекте с постоянным путём учёта.
type identity struct {
	// path — первый успешно разрешённый полный очищенный путь объекта.
	// Не меняется при последующем обнаружении другого написания или ссылки.
	path string

	// info — сведения для проверки файловой идентичности средствами ОС.
	info fs.FileInfo
}

// Inspect проверяет существующий полный путь и возвращает его Entry.
// Относительный path заранее разрешают через Resolve с явной базой.
// Разрешаются файловые ссылки и ссылки каталогов, включая junction Windows.
// Варианты регистра, разделителей, ссылки и жёсткие ссылки одного объекта
// получают одинаковый IdentityPath в пределах Resolver; разные объекты,
// в том числе в каталогах с чувствительностью к регистру, не объединяются.
// Path сохраняет адрес обращения; сведения и доступность проверяются заново.
// Отсутствующий объект, отказ доступа и неразрешимая петля ссылок — ошибки.
// При ошибке Entry не используется. Никакие байты файла не читаются.
func (r *Resolver) Inspect(path string) (Entry, *diagnostic.Error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) {
		return Entry{}, pathError(CodePath, "Для проверки нужен корректный полный путь", path, nil)
	}
	path = filepath.Clean(path)
	resolved, info, err := resolveExisting(path)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Path: path, IdentityPath: r.identify(resolved, info), Info: info}, nil
}

// resolveExisting разрешает ссылки полного пути и получает сведения о цели;
// возвращает очищенный абсолютный путь без изменения регистра для чтения.
func resolveExisting(path string) (string, fs.FileInfo, *diagnostic.Error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) {
		return "", nil, pathError(CodePath, "Для проверки нужен корректный полный путь", path, nil)
	}
	path = filepath.Clean(path)
	isExtendedUNC := func(full string) bool {
		return os.PathSeparator == '\\' && len(full) >= 8 && strings.EqualFold(full[:8], `\\?\UNC\`)
	}
	var active []string
	var resolve func(string) (string, *diagnostic.Error)
	resolve = func(full string) (string, *diagnostic.Error) {
		volume := filepath.VolumeName(full)
		if isExtendedUNC(full) {
			// VolumeName считает томом только \\?\UNC; server/share неделимы.
			parts := strings.SplitN(full[8:], `\`, 3)
			if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
				return "", pathError(CodePath, "UNC-путь должен содержать имя сервера и общего каталога", path, nil)
			}
			volume = full[:8] + parts[0] + `\` + parts[1]
		}
		current := volume + string(os.PathSeparator)
		rest := strings.TrimLeft(full[len(volume):], string(os.PathSeparator))
		for _, part := range strings.Split(rest, string(os.PathSeparator)) {
			if part == "" {
				continue
			}
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if err != nil {
				if len(active) != 0 {
					return "", pathError(CodeLink, "Не удалось разрешить цель файловой ссылки", path, err)
				}
				return "", pathError(CodeAccess, "Не удалось получить сведения об объекте", path, err)
			}
			if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) == 0 {
				continue
			}
			// Junction в Go 1.25 имеет ModeIrregular, но доступен через Readlink.
			target, linkErr := os.Readlink(current)
			if linkErr != nil {
				if info.Mode()&fs.ModeSymlink == 0 &&
					(errors.Is(linkErr, fs.ErrNotExist) || errors.Is(linkErr, syscall.EINVAL)) {
					continue
				}
				return "", pathError(CodeLink, "Не удалось прочитать файловую ссылку", path, linkErr)
			}
			repeated := false
			for _, previous := range active {
				if previous == current {
					repeated = true
					break
				}
			}
			if repeated {
				// Получаем исходную ошибку ОС, не читая содержимое цели.
				if _, statErr := os.Stat(current); statErr != nil {
					return "", pathError(CodeLink, "Обнаружена неразрешимая петля файловых ссылок", path, statErr)
				}
				// Ссылка могла измениться между проверками: используем свежую цель.
				if isExtendedUNC(current) {
					// EvalSymlinks также обходит неполный extended UNC server.
					previous := active
					active = nil
					resolved, resolveErr := resolve(current)
					active = previous
					if resolveErr != nil {
						return "", pathError(CodeLink, "Не удалось разрешить файловые ссылки пути", path, resolveErr.Cause)
					}
					current = resolved
					continue
				}
				resolved, evalErr := filepath.EvalSymlinks(current)
				if evalErr != nil {
					return "", pathError(CodeLink, "Не удалось разрешить файловые ссылки пути", path, evalErr)
				}
				current = resolved
				continue
			}
			targetPath, targetErr := Resolve(filepath.Dir(current), target)
			if targetErr != nil {
				return "", pathError(CodeLink, "Некорректная цель файловой ссылки", path, targetErr)
			}
			// Одна ссылка по разным адресам может иметь разные относительные цели.
			// Петлю определяет повтор адреса в ещё не разрешённой цепочке.
			active = append(active, current)
			resolved, resolveErr := resolve(targetPath)
			active = active[:len(active)-1]
			if resolveErr != nil {
				return "", resolveErr
			}
			current = resolved
		}
		return filepath.Clean(current), nil
	}
	resolved, err := resolve(path)
	if err != nil {
		return "", nil, err
	}
	// Компонентный обход уже разрешил ссылки extended UNC, включая junction.
	// Повторный EvalSymlinks потерял бы границу server/share в walkSymlinks/toNorm.
	if !isExtendedUNC(resolved) {
		var evalErr error
		resolved, evalErr = filepath.EvalSymlinks(resolved)
		if evalErr != nil {
			return "", nil, pathError(CodeLink, "Не удалось разрешить файловые ссылки пути", path, evalErr)
		}
	}
	info, statErr := os.Stat(resolved)
	if statErr != nil {
		return "", nil, pathError(CodeAccess, "Не удалось получить сведения о цели пути", path, statErr)
	}
	return filepath.Clean(resolved), info, nil
}

// identify возвращает закреплённый путь объекта либо регистрирует его впервые.
// Использует идентичность файловой системы, а не только строки без регистра.
func (r *Resolver) identify(resolved string, info fs.FileInfo) string {
	for _, known := range r.identities {
		if os.SameFile(known.info, info) {
			return known.path
		}
	}
	// Закрепляем доступную идентичность ОС сразу, до возможного изменения пути.
	os.SameFile(info, info)
	r.identities = append(r.identities, identity{path: resolved, info: info})
	return resolved
}
