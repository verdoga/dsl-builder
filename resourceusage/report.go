package resourceusage

import "sort"

// Report — независимый снимок учёта ресурсов для превью текущей операции.
// Списки не пересекаются и вместе содержат все учтённые пути без повторов.
// Total равен Used плюс Unused; Used и Unused равны длинам своих списков.
// Это служебные данные, не часть JSON урока или манифеста.
type Report struct {
	// Total — количество всех уникальных учтённых файлов.
	Total int

	// Used — количество уникальных успешно встроенных файлов.
	Used int

	// Unused — количество уникальных файлов без успешного встраивания.
	Unused int

	// UsedPaths — полные нормализованные пути использованных файлов.
	UsedPaths []string

	// UnusedPaths — полные нормализованные пути неиспользованных файлов.
	UnusedPaths []string
}

// Snapshot возвращает согласованные списки и счётчики на момент вызова.
// Списки упорядочены лексикографически по полным строкам путей средствами Go;
// порядок регистрации и объединения реестров на результат не влияет.
// Срезы принадлежат снимку: их изменение не меняет реестр и другие снимки.
// Пустой реестр даёт нулевые счётчики и пустые списки.
// Наличие неиспользованных файлов само по себе не является ошибкой.
// Подпись области статистики и вывод полных путей выполняет пакет превью.
func (r *Registry) Snapshot() Report {
	report := Report{Total: len(r.files)}
	for path, used := range r.files {
		if used {
			report.UsedPaths = append(report.UsedPaths, path)
		} else {
			report.UnusedPaths = append(report.UnusedPaths, path)
		}
	}
	sort.Strings(report.UsedPaths)
	sort.Strings(report.UnusedPaths)
	report.Used = len(report.UsedPaths)
	report.Unused = len(report.UnusedPaths)
	return report
}
