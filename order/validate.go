// Package order проверяет запись веса и сравнивает положительные целые
// произвольной длины без ограничения диапазоном int и без float64.
// Исходные строки, включая ведущие нули, остаются неизменными.
// Наличие веса, синтез, группы, сортировку уроков и конфликты определяет catalog;
// допустимость null в модели проверяет владелец соответствующего контракта.
package order

import "github.com/verdoga/dsl-builder/diagnostic"

// CodeInvalidWeight — код недопустимой записи положительного целого веса.
const CodeInvalidWeight = "order.invalid_weight"

// Validate проверяет всё raw: только цифры ASCII 0–9 и хотя бы одна ненулевая.
// Ведущие нули допустимы; пустая строка и строка только из нулей недопустимы.
// Знаки, дроби, экспонента, пробелы, переводы строк и не-ASCII цифры недопустимы.
// Проверка соответствует записи order в Contract 1.0 и Manifest.schema.json;
// длина числа и его значение не ограничиваются машинным числовым типом.
// Отсутствие поля отличается от пустой строки и обрабатывается вызывающим кодом.
// При успехе возвращает nil, не исправляя и не нормализуя raw.
// При ошибке возвращает diagnostic.Error с CodeInvalidWeight, русской причиной,
// Context равным context и подробностью с Label «Исходный вес», Value равным raw,
// включая пустое значение. Cause отсутствует; неизвестные координаты не выдумываются.
func Validate(raw string, context diagnostic.Context) *diagnostic.Error {
	valid := true
	nonzero := false
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			valid = false
			break
		}
		if raw[i] != '0' {
			nonzero = true
		}
	}
	if valid && nonzero {
		return nil
	}
	return &diagnostic.Error{
		Code:    CodeInvalidWeight,
		Reason:  "Вес должен быть положительным целым числом из цифр ASCII 0–9.",
		Context: context,
		Details: []diagnostic.Detail{{Label: "Исходный вес", Value: raw}},
	}
}
