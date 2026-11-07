package main

// buildVersion — версия выпуска, задаваемая при сборке через -ldflags -X
// main.buildVersion=<версия>. Передаётся console.Run без подмены значения.
// Пустое значение отклоняет console.Run; фиксированной версии по умолчанию нет.
var buildVersion string
