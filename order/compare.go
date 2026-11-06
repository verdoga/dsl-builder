package order

import "strings"

// Compare сравнивает числовые значения left и right без потери точности.
// Предусловие: обе строки успешно проверены Validate; вызывающий код проверяет
// каждую новую или изменённую строку до сравнения и обработки группы.
// Возвращает -1, если left меньше right; 0 при равенстве; 1, если left больше.
// Ведущие нули на результат не влияют: «1», «01» и «001» равны.
// Полные строки сохраняются; сравнение не назначает весов и не сортирует уроки.
// Возвращаемый int содержит только результат сравнения, а не значение веса.
func Compare(left, right string) int {
	left = strings.TrimLeft(left, "0")
	right = strings.TrimLeft(right, "0")

	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
