package filetree

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// Resolve строит полный очищенный Windows-путь без проверки существования.
// Полный путь с диском или UNC используется непосредственно, иначе нужен
// явно переданный полный baseDir; текущий каталог процесса не подставляется.
// Для ресурсов baseDir равен каталогу Entry.Path технического JSON,
// а не document.filePath, корню сборки или IdentityPath файла.
// Разделители приводятся к системной форме, точки и двойные точки очищаются;
// Unicode, пробелы внутри имён и регистр сохраняются, кавычки не снимаются.
// Пустой путь ошибочен. Путь относительно корня диска наследует том baseDir;
// путь вида C:part разрешается от baseDir только при совпадении тома,
// иначе возвращается ошибка без обращения к текущему каталогу другого диска.
// Допустимые длинные и UNC-пути не ограничиваются искусственной длиной.
func Resolve(baseDir string, path string) (string, *diagnostic.Error) {
	if path == "" {
		return "", pathError(CodePath, "Путь не задан", path, nil)
	}
	if strings.IndexByte(path, 0) >= 0 {
		return "", pathError(CodePath, "Путь содержит нулевой символ", path, nil)
	}
	isFull := func(full string) bool {
		if !filepath.IsAbs(full) {
			return false
		}
		unc := filepath.ToSlash(full)
		if !strings.HasPrefix(unc, "//") {
			return true
		}
		unc = unc[2:]
		if strings.HasPrefix(unc, "?/") || strings.HasPrefix(unc, "./") {
			unc = unc[2:]
			if strings.EqualFold(unc, "UNC") {
				return false
			}
			if len(unc) < 4 || !strings.EqualFold(unc[:4], "UNC/") {
				return true
			}
			unc = unc[4:]
		}
		server, share, found := strings.Cut(unc, "/")
		share, _, _ = strings.Cut(share, "/")
		return found && server != "" && share != ""
	}
	clean := func(full string) (string, string) {
		unc := filepath.ToSlash(full)
		if len(unc) >= 8 && strings.EqualFold(unc[:8], "//?/UNC/") {
			server, rest, found := strings.Cut(unc[8:], "/")
			share, _, _ := strings.Cut(rest, "/")
			if found && server != "" && share != "" {
				// Go 1.25 считает томом только \\?\UNC; очищаем хвост
				// отдельно, чтобы .. не удалял сервер или общий каталог.
				volume := filepath.FromSlash(unc[:8+len(server)+1+len(share)])
				tail := unc[len(volume):]
				if tail == "" {
					return volume, volume
				}
				return volume + filepath.Clean(filepath.FromSlash("/"+strings.TrimLeft(tail, "/"))), volume
			}
		}
		cleaned := filepath.Clean(full)
		return cleaned, filepath.VolumeName(cleaned)
	}
	cleaned, volume := clean(path)
	if filepath.IsAbs(cleaned) {
		if !isFull(cleaned) {
			return "", pathError(CodePath, "UNC-путь должен содержать имя сервера и общего каталога", path, nil)
		}
		return cleaned, nil
	}
	if strings.IndexByte(baseDir, 0) >= 0 || !isFull(baseDir) {
		return "", pathError(CodePath, "Для относительного пути нужен полный базовый каталог", path, nil)
	}
	base, baseVolume := clean(baseDir)
	if volume != "" {
		if !strings.EqualFold(volume, baseVolume) {
			return "", pathError(CodePath, "Том относительного пути не совпадает с томом базового каталога", path, nil)
		}
		resolved, _ := clean(base + string(os.PathSeparator) + cleaned[len(volume):])
		return resolved, nil
	}
	if os.IsPathSeparator(cleaned[0]) {
		resolved, _ := clean(baseVolume + cleaned)
		return resolved, nil
	}
	resolved, _ := clean(base + string(os.PathSeparator) + cleaned)
	return resolved, nil
}

// Relative возвращает относительный путь path от полного baseDir.
// Использует адреса обнаружения без разрешения ссылок; разные тома ошибочны.
// Разделители результата приводятся к / для единообразного сравнения;
// исходный регистр сохраняется. Файловую систему функция не читает.
func Relative(baseDir string, path string) (string, *diagnostic.Error) {
	if strings.IndexByte(baseDir, 0) >= 0 || !filepath.IsAbs(baseDir) {
		return "", pathError(CodePath, "Для относительного пути нужен полный базовый каталог", baseDir, nil)
	}
	if strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) {
		return "", pathError(CodePath, "Для сравнения нужен полный путь", path, nil)
	}
	base, baseErr := Resolve("", baseDir)
	if baseErr != nil {
		return "", baseErr
	}
	target, targetErr := Resolve("", path)
	if targetErr != nil {
		return "", targetErr
	}
	volumeName := func(full string) string {
		unc := filepath.ToSlash(full)
		if len(unc) >= 8 && strings.EqualFold(unc[:8], "//?/UNC/") {
			server, rest, _ := strings.Cut(unc[8:], "/")
			share, _, _ := strings.Cut(rest, "/")
			return full[:8+len(server)+1+len(share)]
		}
		return filepath.VolumeName(full)
	}
	// У extended UNC сервер и общий каталог также входят в том,
	// хотя filepath.Rel в Go 1.25 считает их обычными компонентами.
	if !strings.EqualFold(volumeName(base), volumeName(target)) {
		return "", pathError(CodePath, "Том пути не совпадает с томом базового каталога", path, nil)
	}
	if target == filepath.VolumeName(target) {
		target += string(os.PathSeparator)
	}
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return "", pathError(CodePath, "Не удалось вычислить относительный путь от базового каталога", path, err)
	}
	return filepath.ToSlash(relative), nil
}

// ComparePaths сравнивает строки путей сначала после strings.ToLower,
// затем в исходном написании; оба ключа сравниваются лексикографически.
// Перед сравнением разделители приводятся к /; результат равен -1, 0 или 1.
// Локаль Windows и даты файлов не участвуют. Это порядок, не идентичность:
// одно сравнение без регистра не доказывает совпадение файлов.
func ComparePaths(left string, right string) int {
	left = strings.ReplaceAll(left, `\`, "/")
	right = strings.ReplaceAll(right, `\`, "/")
	if comparison := strings.Compare(strings.ToLower(left), strings.ToLower(right)); comparison != 0 {
		return comparison
	}
	return strings.Compare(left, right)
}

// SortRelative возвращает независимый срез entries, упорядоченный
// ComparePaths по Relative(baseDir, Entry.Path); входной срез не изменяет.
// Ошибка вычисления любого относительного пути запрещает частичный результат.
func SortRelative(baseDir string, entries []Entry) ([]Entry, *diagnostic.Error) {
	ordered := make([]struct {
		entry    Entry
		relative string
	}, len(entries))
	for i, entry := range entries {
		relative, err := Relative(baseDir, entry.Path)
		if err != nil {
			return nil, err
		}
		ordered[i].entry = entry
		ordered[i].relative = relative
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ComparePaths(ordered[i].relative, ordered[j].relative) < 0
	})
	result := make([]Entry, len(ordered))
	for i, item := range ordered {
		result[i] = item.entry
	}
	return result, nil
}

// Unique возвращает независимый срез без повторов IdentityPath,
// сохраняя первое вхождение и порядок. Допускает пустой вход.
// Все entries должны быть получены одним Resolver в текущей операции.
// Отбор по имени или расширению выполняют до Unique, чтобы неподходящий
// псевдоним файла не скрыл подходящий. Файловую систему функция не читает.
func Unique(entries []Entry) []Entry {
	result := make([]Entry, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !seen[entry.IdentityPath] {
			seen[entry.IdentityPath] = true
			result = append(result, entry)
		}
	}
	return result
}
