package parsed

import (
	"io"
	"os"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// readBytes полностью читает файл по переданному адресу, закрывает его при
// любом исходе и сообщает ошибки чтения/закрытия с исходной Cause и путём.
// Каталоги не обходит, пути не разрешает и исходник не изменяет.
func readBytes(path string) ([]byte, []*diagnostic.Error) {
	context := diagnostic.Context{Path: path}
	file, err := os.Open(path)
	if err != nil {
		return nil, []*diagnostic.Error{
			inputError(CodeRead, "Не удалось открыть технический JSON для чтения.", "", context, err),
		}
	}

	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	var problems []*diagnostic.Error
	if readErr != nil {
		problems = append(problems, inputError(CodeRead, "Не удалось полностью прочитать технический JSON.", "", context, readErr))
	}
	if closeErr != nil {
		problems = append(problems, inputError(CodeRead, "Не удалось закрыть технический JSON после чтения.", "", context, closeErr))
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return data, nil
}

// prepareUTF8 проверяет корректность всех байтов UTF-8 до JSON-декодирования
// и снимает не более одного BOM только в начале. Не исправляет неверные байты,
// не удаляет BOM в строковых данных, пробелы или переносы внутри JSON.
// Это BOM технического JSON, а не document.hasBom исходного DSL.
func prepareUTF8(data []byte, context diagnostic.Context) ([]byte, *diagnostic.Error) {
	if !utf8.Valid(data) {
		return nil, inputError(CodeUTF8, "Технический JSON содержит некорректную последовательность UTF-8.", "", context, nil)
	}
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		return data[3:], nil
	}
	return data, nil
}
