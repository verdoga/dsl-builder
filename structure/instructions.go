package structure

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// splitInstruction определяет необходимость отдельной ноды только для
// multifill/ordering по наличию content/placeholder элементов объявления.
// Только пустые content.Value без placeholder не создают инструкцию;
// null у content даёт CodeSource, а null у обычного placeholder допустим.
// Не собирает строку, не интерпретирует разметку и не меняет исходный Entry.
// appendContent при true создаёт отдельную NodeInstruction непосредственно
// перед упражнением с тем же родителем и Binding только на его объявление.
// Binding.TextSource передаёт исходную декларацию и пустое тело принятому
// textdata.Build(NodeInstruction, declaration, lines, context).
// Для multifill/ordering основной владелец данных игнорирует инструкцию
// объявления: она уже адресована отдельной ноде и не входит в их data.
// У choice/multichoice/matching возвращает false: инструкция остаётся в исходном
// объявлении и позднее превращается в один data.instruction внутри упражнения.
// Отдельный instr сохраняется независимо от наличия и совпадения инструкции.
func splitInstruction(entry *fragments.Entry, kind lesson.NodeType, state *builder) (bool, *diagnostic.Error) {
	if kind != lesson.NodeMultifill && kind != lesson.NodeOrdering {
		return false, nil
	}

	present := false
	for _, element := range entry.Source.Elements {
		switch element.ElementType {
		case model.ElementTypeContent:
			if element.Value == nil {
				problem := structureError(CodeSource, "Неизвестно семантическое значение инструкции в объявлении упражнения", entry, nil, state)
				problem.Context.Column = element.Start
				return false, problem
			}
			if *element.Value != "" {
				present = true
			}
		case model.ElementTypePlaceholder:
			present = true
		}
	}
	return present, nil
}
