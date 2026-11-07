package moduleio

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
)

// checkJSONEncoding проверяет UTF-8 точных байтов и отсутствие начального BOM.
// Разрешение снять BOM относится к техническому JSON, а не к готовому модулю.
// Неверные байты не заменяются символом замены; raw остаётся неизменным.
func checkJSONEncoding(raw []byte, context diagnostic.Context) *diagnostic.Error {
	if !utf8.Valid(raw) {
		return &diagnostic.Error{Code: CodeEncoding, Reason: "JSON должен содержать корректные байты UTF-8.", Context: context}
	}
	if bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		return &diagnostic.Error{Code: CodeEncoding, Reason: "JSON готового модуля не должен начинаться с BOM.", Context: context}
	}
	return nil
}

// decodeJSONValue читает ровно одно JSON-значение стандартным Decoder с
// UseNumber и требует EOF после допустимых пробелов; второе значение и мусор
// дают CodeJSON. Предусловие — успешная checkJSONEncoding.
// Возвращает стандартное дерево с json.Number, без float64 и ограничения
// чисел машинным int; при ошибке значение nil. Проверок схем здесь нет.
func decodeJSONValue(raw json.RawMessage, context diagnostic.Context) (any, *diagnostic.Error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, &diagnostic.Error{Code: CodeJSON, Reason: "Не удалось прочитать значение JSON.", Context: context, Cause: err}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("после первого значения JSON найдено второе значение")
		}
		return nil, &diagnostic.Error{Code: CodeJSON, Reason: "После единственного значения JSON допускаются только пробелы.", Context: context, Cause: err}
	}
	return value, nil
}

// decodeLesson вызывает checkJSONEncoding и lesson.ValidateJSON, затем
// encoding/json декодирует те же raw в lesson.Lesson через её UnmarshalJSON.
// Требуется ровно одно полное JSON-значение, без хвостового мусора.
// Типизированные Data и числа восстанавливаются существующей моделью lesson;
// неизвестные допустимые свойства остаются в raw для побайтового переноса.
// При любой ошибке возвращает nil; к диагностике добавляет известный контекст,
// сохраняя коды и причины владельца схемы. Значения модели не исправляет.
func decodeLesson(raw json.RawMessage, context diagnostic.Context) (*lesson.Lesson, []*diagnostic.Error) {
	if problem := checkJSONEncoding(raw, context); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		return nil, problems
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		var problem *diagnostic.Error
		if errors.As(err, &problem) {
			copied := *problem
			if copied.Context.Path == "" {
				copied.Context.Path = context.Path
			}
			if copied.Context.FileName == "" {
				copied.Context.FileName = context.FileName
			}
			if copied.Context.LessonID == "" {
				copied.Context.LessonID = context.LessonID
			}
			if copied.Context.PageID == "" {
				copied.Context.PageID = context.PageID
			}
			if copied.Context.Line == 0 {
				copied.Context.Line = context.Line
			}
			if copied.Context.Column == 0 {
				copied.Context.Column = context.Column
			}
			return nil, []*diagnostic.Error{&copied}
		}
		return nil, []*diagnostic.Error{{Code: CodeJSON, Reason: "Не удалось декодировать урок.", Context: context, Cause: err}}
	}
	return &value, nil
}

// decodeManifest вызывает checkJSONEncoding, decodeJSONValue и фактический
// manifest.ValidateShape до декодирования encoding/json в manifest.Manifest.
// ValidateJSON в принятом API manifest отсутствует; проверки формы не дублируются.
// ValidateShape сохраняет различие пропущенных полей, null и пустых массивов
// и отклоняет посторонние имена, включая отличающееся написание регистра.
// json.Number сохраняет точные числа; семантический manifest.Validate далее
// вызывает module.Validate. При любой ошибке результат nil.
func decodeManifest(raw json.RawMessage, context diagnostic.Context) (*manifest.Manifest, []*diagnostic.Error) {
	if problem := checkJSONEncoding(raw, context); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	tree, problem := decodeJSONValue(raw, context)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	if problems := manifest.ValidateShape(tree, context); len(problems) != 0 {
		return nil, problems
	}
	var value manifest.Manifest
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, []*diagnostic.Error{{Code: CodeJSON, Reason: "Не удалось декодировать манифест.", Context: context, Cause: err}}
	}
	return &value, nil
}
