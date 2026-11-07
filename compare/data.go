package compare

import (
	"bytes"
	"encoding/base64"

	"github.com/verdoga/dsl-builder/lesson"
)

// equalDataList сравнивает длину и каждый элемент через equalData в порядке
// массива. Вложенные коллекции используют то же правило без ограничения
// глубины; изменение числа элементов данных меняет собственные поля ноды,
// а не форму дерева nodes и само по себе не заменяет все ноды страницы.
func equalDataList(left []lesson.Data, right []lesson.Data) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !equalData(left[i], right[i]) {
			return false
		}
	}
	return true
}

// equalData сравнивает Type, Role с учётом отсутствия, Value через
// equalDataValue, Answers с сохранением порядка и Hint. Не нормализует
// пробелы, регистр, переносы, HTML, ответы и внутреннюю разметку multifill.
// Ответы не вычисляются; одинаковый текст в разных ролях остаётся различным.
func equalData(left lesson.Data, right lesson.Data) bool {
	if left.Type != right.Type || (left.Role == nil) != (right.Role == nil) {
		return false
	}
	if left.Role != nil && *left.Role != *right.Role {
		return false
	}
	if left.Hint != right.Hint || len(left.Answers) != len(right.Answers) {
		return false
	}
	for i := range left.Answers {
		if left.Answers[i] != right.Answers[i] {
			return false
		}
	}
	return equalDataValue(left, right)
}

// equalDataValue принимает уже проверенные lesson.Validate элементы одного
// Type. Сравнивает string точно; EmbeddedFile через equalEmbeddedFile;
// []lesson.Data через equalDataList. Иные варианты исключены валидацией.
// Сериализация в JSON и сравнение оформления объектов не используются.
func equalDataValue(left lesson.Data, right lesson.Data) bool {
	switch left.Type {
	case lesson.DataText:
		return left.Value.(string) == right.Value.(string)
	case lesson.DataMedia:
		return equalEmbeddedFile(left.Value.(lesson.EmbeddedFile), right.Value.(lesson.EmbeddedFile))
	case lesson.DataCollection:
		return equalDataList(left.Value.([]lesson.Data), right.Value.([]lesson.Data))
	}
	return false
}

// equalEmbeddedFile сравнивает точные FileName, MIMEType и содержимое файла
// через equalEmbeddedContent. Имя и MIME значимы
// даже при тех же байтах. Пути ресурсов и файловые даты недоступны и не нужны;
// файлы не читаются, медиаконтейнеры не анализируются и Base64 не выводится.
func equalEmbeddedFile(left lesson.EmbeddedFile, right lesson.EmbeddedFile) bool {
	return left.FileName == right.FileName && left.MIMEType == right.MIMEType &&
		equalEmbeddedContent(left.Content, right.Content)
}

// equalEmbeddedContent сравнивает декодированные байты проверенного Base64
// средствами стандартной библиотеки. Не зависит от оформления JSON и не
// предполагает дополнительных требований каноничности сверх lesson.Validate.
// Содержимое не перекодируется в модели; пустой файл сравнивается как пустой.
func equalEmbeddedContent(left string, right string) bool {
	leftBytes, err := base64.StdEncoding.DecodeString(left)
	if err != nil {
		return false
	}
	rightBytes, err := base64.StdEncoding.DecodeString(right)
	if err != nil {
		return false
	}
	return bytes.Equal(leftBytes, rightBytes)
}
