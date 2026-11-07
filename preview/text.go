package preview

import "strconv"

// quotedText представляет строку в кавычках с видимыми экранированными
// переносами и управляющими символами. Сохраняет Unicode, регистр, пробелы
// и всё значение; не допускает управляющих последовательностей терминала.
// Используется для названий, путей и иных внешних строк внутри одной строки
// отчёта; экранирование — оформление, не изменение модели.
func quotedText(value string) string {
	return strconv.Quote(value)
}

// optionalText отличает nil («null») от указателя на пустую строку (две
// кавычки); непустые значения представляет через quotedText.
func optionalText(value *string) string {
	if value == nil {
		return "null"
	}
	return quotedText(*value)
}
