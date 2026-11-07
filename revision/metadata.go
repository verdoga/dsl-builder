package revision

import (
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/verdoga/dsl-builder/lesson"
)

// nextRevision прибавляет ровно 1 к математическому целому значению
// проверенного old.Version и возвращает каноническое JSON-число цифрами.
// Например, 5.0 и 5e0 дают 6. Точность и диапазон не ограничиваются
// float64, машинным int или uint64; применяется math/big.
func nextRevision(old json.Number) json.Number {
	revision := integerValue(old)
	revision.Add(revision, big.NewInt(1))
	return json.Number(revision.String())
}

// integerValue получает точное целое из уже проверенного lesson.Validate
// json.Number, включая допустимую дробную и экспоненциальную запись.
// Это служебное представление для арифметики ревизий, не новый валидатор
// схемы. Экспонента также не ограничивается машинным диапазоном.
func integerValue(value json.Number) *big.Int {
	coefficient := string(value)
	exponent := new(big.Int)
	if index := strings.IndexAny(coefficient, "eE"); index >= 0 {
		exponent.SetString(coefficient[index+1:], 10)
		coefficient = coefficient[:index]
	}
	if index := strings.IndexByte(coefficient, '.'); index >= 0 {
		fractionDigits := len(coefficient) - index - 1
		exponent.Sub(exponent, big.NewInt(int64(fractionDigits)))
		coefficient = coefficient[:index] + coefficient[index+1:]
	}
	integer, _ := new(big.Int).SetString(coefficient, 10)
	if integer.Sign() == 0 || exponent.Sign() == 0 {
		return integer
	}
	magnitude := new(big.Int).Abs(exponent)
	power := new(big.Int).Exp(big.NewInt(10), magnitude, nil)
	if exponent.Sign() < 0 {
		return integer.Quo(integer, power)
	}
	return integer.Mul(integer, power)
}

// stampInitial устанавливает Version=1 и CreatedAt через versionTime.
// LineCount остаётся точным значением кандидата из технического JSON;
// количество страниц, нод, переводов строк и строк сериализации не считает.
func stampInitial(result *lesson.Document, at time.Time) {
	result.Version = json.Number("1")
	result.CreatedAt = versionTime(at)
}

// stampUpdate устанавливает уже вычисленную revision, CreatedAt через
// versionTime и точное candidateLineCount. Вызывается один раз только
// при changed=true, даже для одного подтверждённого названия страницы.
func stampUpdate(result *lesson.Document, revision json.Number, candidateLineCount json.Number, at time.Time) {
	result.Version = revision
	result.CreatedAt = versionTime(at)
	result.LineCount = candidateLineCount
}

// versionTime форматирует at в UTC как YYYY-MM-DDTHH:MM:SSZ без долей секунды.
// Не читает системные часы, не использует дату исходного файла или парсера.
// Допустимость четырёхзначного года проверит итоговая lesson.Validate;
// при отсутствии изменений at не используется и не порождает ошибку даты.
func versionTime(at time.Time) string {
	return at.UTC().Format("2006-01-02T15:04:05Z")
}
