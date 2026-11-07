package console

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

// readLine читает одну строку до Enter, снимая только LF и предшествующий CR.
// Сохраняет Unicode, пробелы, кавычки и цифры; конечный фрагмент при EOF
// не считается ответом. EOF возвращается непосредственно, иная ошибка чтения
// через streamError(CodeInput, cause), немедленно и без повторов.
func (d *dialog) readLine() (string, error) {
	line, err := d.input.ReadString('\n')
	if err == io.EOF {
		return "", io.EOF
	}
	if err != nil {
		return "", streamError(CodeInput, err)
	}
	line = strings.TrimSuffix(line, "\n")
	return strings.TrimSuffix(line, "\r"), nil
}

// choose печатает вопрос и непустой список подписей, нумеруя его с 1.
// Все пункты и приглашение находятся на отдельных строках. Возвращает индекс
// с нуля только для показанного списка. Пустой, нечисловой и выходящий за
// диапазон ответ объясняет по-русски и повторяет тот же вопрос с тем же списком.
// Номер меню допустимо разбирать как int; произвольные веса этим API не читают.
func (d *dialog) choose(question string, labels []string) (int, error) {
	var menu strings.Builder
	menu.WriteString(question)
	menu.WriteByte('\n')
	for index, label := range labels {
		fmt.Fprintf(&menu, "%d. %s\n", index+1, displayValue(label))
	}
	menu.WriteString("Ваш выбор:\n")
	text := menu.String()
	for {
		if err := d.writeText(text); err != nil {
			return 0, err
		}
		answer, err := d.readLine()
		if err != nil {
			return 0, err
		}
		answer = strings.TrimSpace(answer)
		digits := answer != ""
		for _, symbol := range answer {
			if symbol < '0' || symbol > '9' {
				digits = false
				break
			}
		}
		number, err := strconv.Atoi(answer)
		if digits && err == nil && number >= 1 && number <= len(labels) {
			return number - 1, nil
		}
		if err := d.writeText("Некорректный выбор. Введите номер пункта из списка и нажмите Enter.\n"); err != nil {
			return 0, err
		}
	}
}

// confirm принимает только «1. Да» и «2. Нет» с Enter. Пустого согласия нет.
// EOF и ошибка потока не превращаются в false с последующим продолжением.
func (d *dialog) confirm(question string) (bool, error) {
	selected, err := d.choose(question, []string{"Да", "Нет"})
	if err != nil {
		return false, err
	}
	return selected == 0, nil
}

// readValue перед свободным значением даёт отдельное числовое меню:
// «1. Ввести значение», «2. Отменить текущий ввод и вернуться».
// Выбор 2 возвращает accepted=false: вызывающий отбрасывает всю локальную
// группу, а не уже введённую часть. Внутри значения «0» всегда обычный текст.
// allowEmpty разрешён только у необязательного юнита, с явным пояснением.
// Пустой обязательный ответ повторяет тот же вопрос. Нормализации названий,
// арифметики весов и проверки путей здесь нет; значение передаётся владельцу.
func (d *dialog) readValue(question string, allowEmpty bool) (value string, accepted bool, err error) {
	for {
		selected, err := d.choose(question, []string{"Ввести значение", "Отменить текущий ввод и вернуться"})
		if err != nil {
			return "", false, err
		}
		if selected == 1 {
			return "", false, nil
		}
		prompt := "Введите значение:\n"
		if allowEmpty {
			prompt = "Введите название юнита (Enter — без юнита):\n"
		}
		if err := d.writeText(prompt); err != nil {
			return "", false, err
		}
		value, err := d.readLine()
		if err != nil {
			return "", false, err
		}
		if value != "" || allowEmpty {
			return value, true, nil
		}
		if err := d.writeText("Пустой ответ недопустим. Введите обязательное значение.\n"); err != nil {
			return "", false, err
		}
	}
}

// writeText записывает готовый текст полностью; короткую запись без ошибки
// считает io.ErrShortWrite. Ошибка немедленно поднимается до Run, без цикла
// повторной печати в неисправный поток и без выполнения следующего действия.
// Возвращаемая ошибка оформлена streamError(CodeOutput, cause).
func (d *dialog) writeText(text string) error {
	written, err := io.WriteString(d.output, text)
	if err == nil && written < len(text) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return streamError(CodeOutput, err)
	}
	return nil
}

// displayValue экранирует управляющие символы и переводы строк во внешнем
// значении, сохраняя Unicode и точные пробелы. Это оформление, не правка данных.
// Применяется к подписям выбора, версии, диагностике и путям результатов.
func displayValue(value string) string {
	var result strings.Builder
	for _, symbol := range value {
		if unicode.IsControl(symbol) || unicode.Is(unicode.Cf, symbol) || symbol == '\u2028' || symbol == '\u2029' {
			quoted := strconv.QuoteRune(symbol)
			result.WriteString(quoted[1 : len(quoted)-1])
		} else {
			result.WriteRune(symbol)
		}
	}
	return result.String()
}

// destinationDirectory снимает ровно одну внешнюю пару двойных кавычек
// только с каталога назначения перед SetDestination: этот метод app вызывает
// filetree.Resolve без inputPath. Не раскрывает escape-последовательности.
// Начальные пути передаются конструкторам без этого вызова: там одну пару
// уже снимает app.inputPath. Повторного снятия и правил source DSL здесь нет.
func destinationDirectory(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}
	return value
}
