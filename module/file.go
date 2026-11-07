package module

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

// File связывает неизменяемый снимок урока с техническим именем и байтами.
// Закрытые поля исключают изменение модели при сохранении устаревших байтов.
// Это состояние сохранности файла, а не новая модель lesson или запись Entry.
type File struct {
	// name — назначенное имя; после создания снимка не меняется.
	name string
	// value — независимый снимок проверенного урока, недоступный для правки извне.
	value *lesson.Lesson
	// original — точные несжатые байты загруженной записи JSON; nil у новых
	// и заменённых уроков. Не ZIP-заголовки и не повторная сериализация модели.
	original []byte
	// context — источник урока: технический JSON либо архив и имя записи.
	context diagnostic.Context
}

// NewFile проверяет имя через identity.ParseFileName и модель через
// lesson.Validate, затем сохраняет независимый снимок через cloneLesson.
// Имя заранее назначено AssignFileNames. original остаётся nil: moduleio
// сериализует модель стандартным JSON с отступом в два пробела без BOM.
// Не генерирует ID, даты или ревизии. Ошибка даёт nil File без частичного успеха.
func NewFile(fileName string, value *lesson.Lesson, context diagnostic.Context) (*File, []*diagnostic.Error) {
	context.FileName = fileName
	if value != nil && value.Document.ID != "" {
		context.LessonID = value.Document.ID
	}
	var problems []*diagnostic.Error
	if _, problem := identity.ParseFileName(fileName); problem != nil {
		problem.Context = context
		problems = append(problems, problem)
	}
	problems = append(problems, lesson.Validate(value, context)...)
	if len(problems) != 0 {
		return nil, problems
	}
	return &File{name: fileName, value: cloneLesson(value), context: context}, nil
}

// LoadedFile сохраняет проверенный moduleio урок и точную копию его raw.
// Предусловие: moduleio проверил UTF-8, целостность ZIP, lesson.ValidateJSON
// и декодировал value именно из raw; повторного разбора JSON здесь нет.
// Координирует NewFile и attachOriginal; не сериализует и не исправляет raw.
func LoadedFile(fileName string, value *lesson.Lesson, raw []byte, context diagnostic.Context) (*File, []*diagnostic.Error) {
	snapshot, problems := NewFile(fileName, value, context)
	if len(problems) != 0 {
		return nil, problems
	}
	loaded, problem := attachOriginal(snapshot, raw)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	return loaded, nil
}

// attachOriginal возвращает новый снимок с копией непустых raw. Пустые байты
// дают CodeOriginalBytes: корректный JSON урока не может быть пустым файлом.
// Вызывается только LoadedFile после установления связи raw и value в moduleio.
func attachOriginal(value *File, raw []byte) (*File, *diagnostic.Error) {
	if len(raw) == 0 {
		return nil, &diagnostic.Error{
			Code: CodeOriginalBytes, Reason: "Отсутствуют исходные байты загруженного урока.",
			Context: value.Context(),
		}
	}
	if value == nil || value.value == nil {
		return nil, &diagnostic.Error{
			Code: CodeFile, Reason: "Отсутствует снимок загруженного урока.",
			Context: value.Context(),
		}
	}
	return &File{
		name: value.name, value: cloneLesson(value.value),
		original: append([]byte(nil), raw...), context: value.context,
	}, nil
}

// ReplaceFile создаёт независимый файл с прежним именем и готовым уроком
// после revision. Нулевые previous, previous.value или value дают CodeFile.
// Проверяет точное совпадение Document.ID со старым написанием,
// затем вызывает NewFile. original нового снимка всегда nil; старый целиком
// сохранён. Другой ID даёт CodeIdentityChange; любая ошибка — nil File.
// Вызывается только для принятого изменения содержимого. При изменении одного
// манифеста или отсутствии изменений app сохраняет прежний File целиком.
// Синхронизацию записи выполняет app через catalog.SyncLesson с итоговым Document.
func ReplaceFile(previous *File, value *lesson.Lesson, context diagnostic.Context) (*File, []*diagnostic.Error) {
	if previous != nil {
		context.FileName = previous.name
		if previous.value != nil && previous.value.Document.ID != "" {
			context.LessonID = previous.value.Document.ID
		}
	}
	if value != nil && value.Document.ID != "" {
		context.LessonID = value.Document.ID
	}
	if previous == nil || previous.value == nil || value == nil {
		return nil, []*diagnostic.Error{{
			Code: CodeFile, Reason: "Для замены нужны прежний файл и оба снимка урока.",
			Context: context,
		}}
	}
	if previous.value.Document.ID != value.Document.ID {
		return nil, []*diagnostic.Error{{
			Code: CodeIdentityChange, Reason: "ID заменяемого урока должен точно совпадать с сохранённым написанием.",
			Context: context,
			Related: []diagnostic.Context{previous.Context(), context},
			Details: []diagnostic.Detail{
				{Label: "Поле JSON", Value: "/document/id"},
				{Label: "Сохранённый ID", Value: previous.value.Document.ID},
				{Label: "ID нового урока", Value: value.Document.ID},
			},
		}}
	}
	return NewFile(previous.name, value, context)
}

// FileName возвращает техническое имя снимка; nil получатель даёт пустую строку.
func (value *File) FileName() string {
	if value == nil {
		return ""
	}
	return value.name
}

// Lesson возвращает независимую копию модели через cloneLesson;
// правка этой копии не меняет снимок и его байты. nil получатель даёт nil.
func (value *File) Lesson() *lesson.Lesson {
	if value == nil {
		return nil
	}
	return cloneLesson(value.value)
}

// OriginalBytes возвращает копию точных байтов либо nil. Ненулевой результат
// moduleio переносит без пересериализации; nil требует сериализовать Lesson.
// При изменениях лишь манифеста байты всех прежних файлов остаются доступны.
func (value *File) OriginalBytes() []byte {
	if value == nil || value.original == nil {
		return nil
	}
	return append([]byte{}, value.original...)
}

// Context возвращает известный источник с именем и ID снимка; nil даёт нулевой
// контекст. Не подставляет document.filePath или вымышленные координаты.
func (value *File) Context() diagnostic.Context {
	if value == nil {
		return diagnostic.Context{}
	}
	context := value.context
	if value.name != "" {
		context.FileName = value.name
	}
	if value.value != nil && value.value.Document.ID != "" {
		context.LessonID = value.value.Document.ID
	}
	return context
}
