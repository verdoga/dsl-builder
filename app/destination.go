package app

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Destination — сведения о цели для обязательного показа до подтверждения.
type Destination struct {
	// Directory — полный существующий каталог, введённый отдельно от имени.
	Directory string
	// FileName — проверенное имя ZIP, без разделителей каталогов.
	FileName string
	// Path — окончательный полный путь, показываемый перед согласием.
	Path string
	// Exists — цель существует и является обычным файлом.
	Exists bool
}

// targetState связывает показанную цель с наблюдением файловой системы.
type targetState struct {
	// destination — сведения, показываемые пользователю.
	destination Destination
	// info — исходная идентичность, размер и дата; nil при отсутствии.
	info os.FileInfo
}

// SetDestination разрешён только при Create. Принимает каталог и имя отдельно,
// вызывает archiveName, filetree.Resolve и inspectDestination. Успех публикует
// цель, отзывает прежнее окончательное согласие и требует нового Snapshot.
// Ошибка ввода не меняет прежнюю цель. Для Update/Add целью служит archivePath.
func (op *Operation) SetDestination(token ReviewToken, directory string, name string) []*diagnostic.Error {
	if issue := op.checkReview(token); issue != nil {
		return []*diagnostic.Error{issue}
	}
	if op.mode != Create {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Выбор назначения доступен только при создании модуля."}}
	}
	fileName, issue := archiveName(name)
	if issue != nil {
		return []*diagnostic.Error{issue}
	}
	resolved, issue := inputPath(op.baseDir, directory)
	if issue != nil {
		return []*diagnostic.Error{issue}
	}
	path, issue := filetree.Resolve(resolved, fileName)
	if issue != nil {
		return []*diagnostic.Error{issue}
	}
	target, issue := inspectDestination(path)
	if issue != nil {
		return []*diagnostic.Error{issue}
	}
	op.target = target
	op.invalidateReview()
	return nil
}

// archiveName проверяет самостоятельное имя Windows: непустое, без / и \ ,
// тома, управляющих символов, < > : " | ? *, крайних точки/пробела и имён
// устройств CON/PRN/AUX/NUL/CONIN$/CONOUT$/COM/LPT с зарезервированными
// номерами, включая допустимые Windows надстрочные варианты, независимо от
// регистра и расширения. Не обрезает ошибочные знаки и не исправляет ввод.
// Существующий суффикс .zip любого регистра сохраняет; иначе добавляет .zip.
// Имя проверяется также после добавления расширения; длина искусственно не ограничена.
func archiveName(name string) (string, *diagnostic.Error) {
	invalid := func(reason string) (string, *diagnostic.Error) {
		return "", &diagnostic.Error{Code: CodeArchiveName, Reason: reason,
			Details: []diagnostic.Detail{{Label: "Имя архива", Value: name}}}
	}
	validate := func(candidate string) bool {
		if candidate == "" || !utf8.ValidString(candidate) || filepath.VolumeName(candidate) != "" ||
			strings.HasPrefix(candidate, ".") || strings.HasPrefix(candidate, " ") ||
			strings.ContainsAny(candidate, `/\<>:"|?*`) || strings.HasSuffix(candidate, ".") || strings.HasSuffix(candidate, " ") {
			return false
		}
		for _, char := range candidate {
			if unicode.IsControl(char) {
				return false
			}
		}
		base, _, _ := strings.Cut(candidate, ".")
		base = strings.ToUpper(strings.TrimRight(base, " "))
		switch base {
		case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
			return false
		}
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			number := base[3:]
			if len(number) == 1 && number[0] >= '1' && number[0] <= '9' || number == "¹" || number == "²" || number == "³" {
				return false
			}
		}
		return true
	}
	if !validate(name) {
		return invalid("Имя архива недопустимо в Windows; введите отдельное имя без пути и зарезервированных обозначений.")
	}
	result := name
	if !strings.EqualFold(filepath.Ext(result), ".zip") {
		result += ".zip"
	}
	if !validate(result) {
		return invalid("Имя архива с расширением ZIP недопустимо в Windows.")
	}
	return result, nil
}

// inspectDestination проверяет существование каталога и os.Lstat цели;
// отличает os.IsNotExist от отказа доступа, запрещает каталог/ссылку вместо ZIP.
// Не создаёт каталог или файл; возвращает полный показанный путь и os.FileInfo.
func inspectDestination(path string) (*targetState, *diagnostic.Error) {
	failure := func(reason string, cause error) (*targetState, *diagnostic.Error) {
		return nil, &diagnostic.Error{Code: CodeDestination, Reason: reason,
			Context: diagnostic.Context{Path: path}, Cause: cause}
	}
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) {
		return failure("Для назначения нужен корректный полный путь.", nil)
	}
	path = filepath.Clean(path)
	directory := filepath.Dir(path)
	parent, err := os.Stat(directory)
	if err != nil {
		return failure("Не удалось проверить существующий каталог назначения.", err)
	}
	if !parent.IsDir() {
		return failure("Родительский путь назначения должен быть каталогом.", nil)
	}
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return failure("Не удалось проверить целевой файл.", err)
	}
	target := &targetState{destination: Destination{
		Directory: directory, FileName: filepath.Base(path), Path: path,
	}}
	if err != nil {
		return target, nil
	}
	if !info.Mode().IsRegular() {
		return failure("Цель должна быть обычным файлом, а не каталогом, ссылкой или специальным устройством.", nil)
	}
	if !os.SameFile(info, info) {
		return failure("Не удалось сохранить файловую идентичность цели.", nil)
	}
	target.destination.Exists = true
	target.info = info
	return target, nil
}

// checkDestination сверяет текущую цель с показанным наблюдением: наличие,
// os.SameFile, размер и время. При изменении отзывает согласие; Update/Add
// требует повторного анализа архива, Create — нового показа цели и согласия.
// Дополняет контроль safewrite, начинающийся только с его собственного вызова.
func (op *Operation) checkDestination() *diagnostic.Error {
	if op == nil {
		return &diagnostic.Error{Code: CodeState, Reason: "Операция не задана."}
	}
	if op.target == nil {
		op.saving = nil
		return &diagnostic.Error{Code: CodeDestination, Reason: "Цель записи ещё не выбрана."}
	}
	changed := func(cause error) *diagnostic.Error {
		issue := &diagnostic.Error{Code: CodeTargetChanged,
			Reason:  "Целевой файл изменился после проверки; требуется заново показать цель и подтвердить запись.",
			Context: diagnostic.Context{Path: op.target.destination.Path}, Cause: cause}
		if op.mode == Update || op.mode == Add {
			issue.Reason = "Исходный архив изменился после чтения; требуется повторный анализ."
			op.analyzed = false
			op.issues = append(op.issues, issue)
		}
		op.invalidateReview()
		return issue
	}
	current, issue := inspectDestination(op.target.destination.Path)
	if issue != nil {
		return changed(issue)
	}
	same := func(previous *targetState) bool {
		if previous == nil || previous.destination.Path != current.destination.Path ||
			previous.destination.Exists != current.destination.Exists {
			return false
		}
		if previous.info == nil || current.info == nil {
			return previous.info == nil && current.info == nil
		}
		return os.SameFile(previous.info, current.info) && previous.info.Size() == current.info.Size() &&
			previous.info.ModTime().Equal(current.info.ModTime())
	}
	if !same(op.target) || op.saving != nil && !same(op.saving.target) {
		return changed(nil)
	}
	return nil
}

// inputPath снимает одну внешнюю пару кавычек интерактивного пути и вызывает
// filetree.Resolve с явной baseDir. Не используется для source внутри JSON.
func inputPath(baseDir string, value string) (string, *diagnostic.Error) {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	return filetree.Resolve(baseDir, value)
}
