# Analyze fixtures

Все JSON создаются в t.TempDir. Неизменённые golden/1..7 parsed.json служат
прямым входом; TXT не разбирается. 1/3 с исходными resourceDirs=[] обязаны
дать resourcefind.not_found. Отдельные synthetic media JSON получают явные
resourceDirs ДО parsed.Load; небольшие реальные байты создаются рядом.

analyzeFixtureArchive использует настоящие parsed.Load → convert.Build →
module.NewFile → catalog.Create/Export → moduleio.Write → moduleio.Read.
Имена l002/l009 оставляют намеренные пробелы нумерации; пробелы перед JSON
в ZIP проверяют сохранение точных OriginalBytes. Это корректный модуль,
а не legacy repair fixture. Loaded Input и индексы тесты не изменяют.
