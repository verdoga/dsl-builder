package console

import (
	"errors"
	"fmt"
	"io"

	"github.com/verdoga/dsl-builder/diagnostic"
)

const (
	// CodeConfiguration — отсутствует обязательная база, версия или поток.
	CodeConfiguration = "console.configuration"
	// CodeInput — потеря входного потока кроме штатного EOF.
	CodeInput = "console.input"
	// CodeOutput — ошибка вывода либо неполная запись в переданный поток.
	CodeOutput = "console.output"
	// CodePreview — переданный подготовленный снимок невозможно полностью показать.
	CodePreview = "console.preview"
)

// PrintError — общий публичный вывод ошибки, в том числе ошибки main до Run
// (получение базового каталога/версии). main передаёт stderr и обычный error,
// не импортируя diagnostic. nil ошибки ничего не печатает. Не завершает процесс.
// Для обычной причины добавляет русское «Не удалось выполнить действие».
// При сбое самого вывода возвращает errors.Join исходной и ошибки потока,
// обёрнутой streamError(CodeOutput, cause), без рекурсивной попытки напечатать
// её туда же. При nil output возвращает CodeConfiguration без паники.
func PrintError(output io.Writer, err error) error {
	if err == nil {
		return nil
	}
	if output == nil {
		return &diagnostic.Error{Code: CodeConfiguration, Reason: "Не задан поток вывода диагностики."}
	}
	if failure := writeErrorTree(output, err); failure != nil {
		return errors.Join(err, failure)
	}
	return nil
}

// showIssues выводит все ненулевые diagnostic.Error по порядку через PrintError.
// Успешный показ возвращает nil: предметные блокировки остаются в app.
// При сбое вывода возвращает errors.Join всех исходных issues и сбоя;
// непоказанные причины не теряются. Typed nil не добавляет в объединение.
func (d *dialog) showIssues(issues []*diagnostic.Error) error {
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		if failure := PrintError(d.output, issue); failure != nil {
			return errors.Join(joinedIssues(issues), failure)
		}
	}
	return nil
}

// writeErrorTree представляет текущую ошибку и каждую дочернюю причину.
// Проверяет именно текущий узел как *diagnostic.Error, отдельно обходит
// Unwrap() []error и Unwrap() error: один errors.As для всего Join запрещён,
// поскольку он скрыл бы вторую ветвь. Родительскую Reason не заменяет Cause.
// Каждый узел сохраняет свой контекст; общий текст Join не дублируется вместо
// дочерних структур. Предупреждения парсера не читает: parsed уже отобрал ошибки.
func writeErrorTree(output io.Writer, err error) error {
	if err == nil {
		return nil
	}
	if issue, ok := err.(*diagnostic.Error); ok {
		return writeDiagnostic(output, issue)
	}
	if branches, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range branches.Unwrap() {
			if failure := writeErrorTree(output, cause); failure != nil {
				return failure
			}
		}
		return nil
	}
	d := dialog{output: output}
	if failure := d.writeText("Не удалось выполнить действие: " + displayValue(err.Error()) + "\n"); failure != nil {
		return failure
	}
	if parent, ok := err.(interface{ Unwrap() error }); ok {
		return writeErrorTree(output, parent.Unwrap())
	}
	return nil
}

// writeDiagnostic выводит Code, русскую Reason, Context, все Related с
// нумерацией участников и все Details в исходном порядке, включая повторные
// Label и пустые Value. Cause обходит writeErrorTree отдельно без потери ветвей.
func writeDiagnostic(output io.Writer, issue *diagnostic.Error) error {
	if issue == nil {
		return nil
	}
	d := dialog{output: output}
	if failure := d.writeText("Код: " + displayValue(issue.Code) + "\nПричина: " + displayValue(issue.Reason) + "\n"); failure != nil {
		return failure
	}
	if failure := writeContext(output, issue.Context); failure != nil {
		return failure
	}
	for index, context := range issue.Related {
		if failure := d.writeText(fmt.Sprintf("Участник %d:\n", index+1)); failure != nil {
			return failure
		}
		if failure := writeContext(output, context); failure != nil {
			return failure
		}
	}
	if failure := writeDetails(output, issue.Details); failure != nil {
		return failure
	}
	return writeErrorTree(output, issue.Cause)
}

// writeContext выводит все известные Path, FileName, LessonID, PageID,
// Line и Column с отдельными русскими подписями. Не подставляет неизвестное,
// document.filePath и не обрезает длинные пути/ID; значения экранирует для вывода.
func writeContext(output io.Writer, context diagnostic.Context) error {
	details := make([]diagnostic.Detail, 0, 6)
	for _, detail := range []diagnostic.Detail{
		{Label: "Путь", Value: context.Path},
		{Label: "Техническое имя", Value: context.FileName},
		{Label: "ID урока", Value: context.LessonID},
		{Label: "ID страницы", Value: context.PageID},
	} {
		if detail.Value != "" {
			details = append(details, detail)
		}
	}
	if context.Line != 0 {
		details = append(details, diagnostic.Detail{Label: "Строка", Value: fmt.Sprint(context.Line)})
	}
	if context.Column != 0 {
		details = append(details, diagnostic.Detail{Label: "Колонка", Value: fmt.Sprint(context.Column)})
	}
	return writeDetails(output, details)
}

// writeDetails выводит каждую пару Label/Value без анализа её предметного
// смысла, числового преобразования, сокращения или добавления рекомендаций.
func writeDetails(output io.Writer, details []diagnostic.Detail) error {
	d := dialog{output: output}
	for _, detail := range details {
		if failure := d.writeText(displayValue(detail.Label) + ": " + displayValue(detail.Value) + "\n"); failure != nil {
			return failure
		}
	}
	return nil
}

// joinedIssues сохраняет исходные ненулевые *diagnostic.Error через errors.Join.
// Это перевод среза в стандартное дерево причин, не новая модель ошибок.
func joinedIssues(issues []*diagnostic.Error) error {
	causes := make([]error, 0, len(issues))
	for _, issue := range issues {
		if issue != nil {
			causes = append(causes, issue)
		}
	}
	return errors.Join(causes...)
}

// streamError создаёт diagnostic.Error с кодом ввода/вывода, русской Reason
// и исходной Cause. Не выдумывает файловый путь для стандартного потока.
func streamError(code string, cause error) *diagnostic.Error {
	reason := "Не удалось прочитать входной поток."
	if code == CodeOutput {
		reason = "Не удалось полностью записать данные в поток вывода."
	}
	return &diagnostic.Error{Code: code, Reason: reason, Cause: cause}
}

// presentationError оборачивает ошибку preview в diagnostic.Error CodePreview
// с русской причиной и сохранённой Cause; несколько причин остаются errors.Join.
// Ошибку потока этой функцией не подменяют.
func presentationError(cause error) *diagnostic.Error {
	return &diagnostic.Error{Code: CodePreview, Reason: "Не удалось полностью показать подготовленный снимок.", Cause: cause}
}

// isStreamFailure проверяет каждую ветвь error на console.input/console.output
// и непосредственный io.EOF от readLine. Не ограничивается первым errors.As
// внутри Join. True запрещает продолжение диалога и любое следующее действие;
// ошибки представления без сбоя потока допускают меню безопасного возврата.
func isStreamFailure(err error) bool {
	if err == io.EOF {
		return true
	}
	var visit func(error) bool
	visit = func(current error) bool {
		if current == nil {
			return false
		}
		if issue, ok := current.(*diagnostic.Error); ok {
			if issue == nil {
				return false
			}
			if issue.Code == CodeInput || issue.Code == CodeOutput {
				return true
			}
		}
		switch cause := current.(type) {
		case interface{ Unwrap() []error }:
			for _, branch := range cause.Unwrap() {
				if visit(branch) {
					return true
				}
			}
		case interface{ Unwrap() error }:
			return visit(cause.Unwrap())
		}
		return false
	}
	return visit(err)
}
