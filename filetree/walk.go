package filetree

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// ReadDir перечисляет непосредственные дочерние объекты полного root.
// root обязан существовать и быть каталогом. Каждый объект проходит Inspect;
// ссылки разрешаются, но дочерние каталоги не обходятся.
// Возвращает файлы и каталоги по ComparePaths относительных имён.
// Разные адреса одного объекта здесь сохранены: вызывающий код сначала
// отбирает нужные имена, затем применяет Unique. Пустой каталог допустим.
// Ошибка доступа, чтения или разрешения любой записи возвращает nil и ошибку.
func (r *Resolver) ReadDir(root string) ([]Entry, *diagnostic.Error) {
	directory, err := r.Inspect(root)
	if err != nil {
		return nil, err
	}
	if !directory.Info.IsDir() {
		return nil, pathError(CodeKind, "Для перечисления нужен каталог", directory.Path, nil)
	}
	children, readErr := os.ReadDir(directory.Path)
	if readErr != nil {
		return nil, pathError(CodeAccess, "Не удалось прочитать каталог", directory.Path, readErr)
	}
	sort.Slice(children, func(i, j int) bool {
		return ComparePaths(children[i].Name(), children[j].Name()) < 0
	})
	entries := make([]Entry, 0, len(children))
	for _, child := range children {
		entry, err := r.Inspect(filepath.Join(directory.Path, child.Name()))
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Walk рекурсивно перечисляет обычные файлы полного каталога root.
// Глубина искусственно не ограничивается. Включает файлы всех расширений,
// если include равен nil; иначе include проверяет каждое файловое обращение
// до устранения повторов. Функция отбора не меняет Resolver и файловую систему.
// Так resourcefind может отбирать имя и расширение до объединения псевдонимов,
// а отдельный вызов с nil — получать все файлы области для учёта ресурсов.
// Каталоги проходят независимо от фильтра; ссылки каталогов обходятся,
// повторно встреченный каталог, включая ссылку на предка, не обходится снова.
// Неразрешимая петля ссылок и любая ошибка доступа возвращают nil и ошибку;
// обычная обратная ссылка на уже известный каталог не является ошибкой.
// Необычный объект, не являющийся файлом или каталогом, даёт явную ошибку.
// Обход детерминирован по ComparePaths дочерних имён; каждый каталог
// перечисляется по первому такому адресу. Даты не учитываются.
// Результат сортируется относительно выбранного root и объединяется по
// IdentityPath после отбора. Пустой результат допустим для ресурсной области.
// Повторный вызов с тем же Resolver выполняет новый полный обход;
// посещённые каталоги не сохраняются между вызовами.
// Для нескольких resourceDirs вызывают Walk для каждого каталога и Unique
// для объединения результатов; ни один явно указанный каталог не пропускают
// без проверки доступности из-за совпадения или пересечения областей.
func (r *Resolver) Walk(root string, include func(Entry) bool) ([]Entry, *diagnostic.Error) {
	directory, err := r.Inspect(root)
	if err != nil {
		return nil, err
	}
	if !directory.Info.IsDir() {
		return nil, pathError(CodeKind, "Для обхода нужен каталог", directory.Path, nil)
	}
	state := traversal{
		pending:     []Entry{directory},
		directories: make(map[string]bool),
	}
	for len(state.pending) != 0 {
		last := len(state.pending) - 1
		next := state.pending[last]
		state.pending = state.pending[:last]
		if err := r.visitDirectory(&state, next, include); err != nil {
			return nil, err
		}
	}
	entries, err := SortRelative(directory.Path, state.files)
	if err != nil {
		return nil, err
	}
	return Unique(entries), nil
}

// traversal содержит только состояние одного обхода; реестром ресурсов не служит.
type traversal struct {
	// pending — необработанные каталоги, обеспечивающие обход без лимита глубины.
	pending []Entry

	// directories — IdentityPath уже перечисленных каталогов против повторов и циклов.
	directories map[string]bool

	// files — прошедшие отбор файлы до итоговой сортировки и устранения повторов.
	files []Entry
}

// visitDirectory перечисляет один каталог через ReadDir и передаёт записи
// для обработки; уже посещённый IdentityPath повторно не перечисляет.
func (r *Resolver) visitDirectory(state *traversal, directory Entry, include func(Entry) bool) *diagnostic.Error {
	if state.directories[directory.IdentityPath] {
		return nil
	}
	entries, err := r.ReadDir(directory.Path)
	if err != nil {
		return err
	}
	state.directories[directory.IdentityPath] = true
	first := len(state.pending)
	for _, entry := range entries {
		if err := visitEntry(state, entry, include); err != nil {
			return err
		}
	}
	// В стеке первым обрабатывается меньшее имя; соседние ветви ждут
	// завершения текущей, поэтому первый адрес каталога детерминирован.
	for left, right := first, len(state.pending)-1; left < right; left, right = left+1, right-1 {
		state.pending[left], state.pending[right] = state.pending[right], state.pending[left]
	}
	return nil
}

// visitEntry добавляет каталог в очередь либо обычный файл после отбора
// в state.files; сообщает о неподдерживаемом виде файлового объекта.
func visitEntry(state *traversal, entry Entry, include func(Entry) bool) *diagnostic.Error {
	if entry.Info.IsDir() {
		state.pending = append(state.pending, entry)
		return nil
	}
	if !entry.Info.Mode().IsRegular() {
		return pathError(CodeKind, "Объект не является обычным файлом или каталогом", entry.Path, nil)
	}
	if include == nil || include(entry) {
		state.files = append(state.files, entry)
	}
	return nil
}
