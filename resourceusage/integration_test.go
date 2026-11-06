package resourceusage_test

import (
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestIntegrationCurrentOperationCombinesLessonUsage(t *testing.T) {
	const (
		studentAudio  = `C:\Учебные материалы\U1\SB\Audio\Трек 01.mp3`
		unsupported   = `C:\Учебные материалы\U1\SB\Audio\notes.bin`
		shared        = `C:\Учебные материалы\U1\Shared\intro.mp3`
		workbookIdle  = `C:\Учебные материалы\U1\WB\Audio\Unused.mp3`
		workbookAudio = `C:\Учебные материалы\U1\WB\Audio\Трек 01.mp3`
		externalBody  = `D:\Внешние ресурсы\Видео 1.mp4`
		externalSRT   = `D:\Внешние ресурсы\Видео 1.srt`
		externalTXT   = `D:\Внешние ресурсы\Видео 1.txt`
		externalVTT   = `D:\Внешние ресурсы\Видео 1.vtt`
	)

	for _, reverse := range []bool{false, true} {
		name := "source_order"
		if reverse {
			name = "reverse_order"
		}
		t.Run(name, func(t *testing.T) {
			var student, workbook, video resourceusage.Registry
			student.Register([]string{shared, unsupported, studentAudio})
			student.MarkUsed([]string{studentAudio})
			// Общий путь пока не востребован этим уроком.
			workbook.Register([]string{workbookAudio, shared, workbookIdle})
			workbook.MarkUsed([]string{shared, workbookAudio})
			workbook.MarkUsed([]string{shared})
			// Готовый выбор вне resourceDirs включает только основной файл
			// и его сопутствующие файлы, без прочих соседей каталога.
			selection := []string{externalBody, externalTXT, externalVTT, externalSRT}
			video.Register(selection)
			video.MarkUsed(selection)

			lessons := []*resourceusage.Registry{&student, &workbook, &video}
			if reverse {
				slices.Reverse(lessons)
			}
			var operation resourceusage.Registry
			for _, lesson := range lessons {
				operation.Merge(lesson)
			}
			want := resourceusage.Report{
				Total: 9, Used: 7, Unused: 2,
				UsedPaths: []string{
					studentAudio, shared, workbookAudio,
					externalBody, externalSRT, externalTXT, externalVTT,
				},
				UnusedPaths: []string{unsupported, workbookIdle},
			}
			ruIntegrationAssertReport(t, operation.Snapshot(), want)
			// Снимок одного урока сохраняет локальный смысл общего пути.
			ruIntegrationAssertReport(t, student.Snapshot(), resourceusage.Report{
				Total: 3, Used: 1, Unused: 2,
				UsedPaths: []string{studentAudio}, UnusedPaths: []string{unsupported, shared},
			})
		})
	}
}

func TestIntegrationUnsuccessfulAttemptDoesNotBecomeUsage(t *testing.T) {
	const (
		body      = `C:\Current\Video\clip.mp4`
		script    = `C:\Current\Video\clip.txt`
		subtitles = `C:\Current\Video\clip.vtt`
		discarded = `C:\Failed lesson\already embedded.mp3`
	)
	var current resourceusage.Registry
	selection := []string{body, script, subtitles}
	current.Register(selection)
	// Попытка чтения/включения завершилась неуспешно: MarkUsed не вызывается.
	var operation resourceusage.Registry
	operation.Merge(&current)
	ruIntegrationAssertReport(t, operation.Snapshot(), resourceusage.Report{
		Total: 3, Unused: 3, UnusedPaths: []string{body, script, subtitles},
	})

	// Реестр урока с поздней ошибкой не публикуется в общую операцию.
	var failedLesson resourceusage.Registry
	failedLesson.Register([]string{discarded})
	failedLesson.MarkUsed([]string{discarded})

	// Тот же успешный выбор в другом уроке делает все три пути использованными.
	var successfulLesson resourceusage.Registry
	successfulLesson.Register(selection)
	successfulLesson.MarkUsed(selection)
	operation.Merge(&successfulLesson)
	ruIntegrationAssertReport(t, operation.Snapshot(), resourceusage.Report{
		Total: 3, Used: 3, UsedPaths: []string{body, script, subtitles},
	})
	ruIntegrationAssertReport(t, current.Snapshot(), resourceusage.Report{
		Total: 3, Unused: 3, UnusedPaths: []string{body, script, subtitles},
	})
}

func TestIntegrationNewAnalysisAndPreviewReportsStayIndependent(t *testing.T) {
	const (
		oldUsed       = `C:\Previous operation\old.mp3`
		oldUnused     = `C:\Previous operation\unused.bin`
		currentUsed   = `C:\Текущий исходник\новый файл.mp3`
		currentUnused = `C:\Текущий исходник\справка.pdf`
	)
	var previous resourceusage.Registry
	previous.Register([]string{oldUsed, oldUnused})
	previous.MarkUsed([]string{oldUsed})

	// Новый реестр получает только ресурсы текущего анализа.
	var lesson, operation resourceusage.Registry
	lesson.Register([]string{currentUnused, currentUsed})
	lesson.MarkUsed([]string{currentUsed})
	operation.Merge(&lesson)
	want := resourceusage.Report{
		Total: 2, Used: 1, Unused: 1,
		UsedPaths: []string{currentUsed}, UnusedPaths: []string{currentUnused},
	}
	preview := operation.Snapshot()
	otherPreview := operation.Snapshot()
	ruIntegrationAssertReport(t, preview, want)

	// Изменение исходного урока после Merge не меняет готовое превью.
	lesson.MarkUsed([]string{currentUnused})
	preview.UsedPaths[0] = oldUsed
	preview.UnusedPaths[0] = oldUnused
	ruIntegrationAssertReport(t, otherPreview, want)
	ruIntegrationAssertReport(t, operation.Snapshot(), want)

	operation.Merge(&lesson)
	ruIntegrationAssertReport(t, operation.Snapshot(), resourceusage.Report{
		Total: 2, Used: 2, UsedPaths: []string{currentUsed, currentUnused},
	})
	ruIntegrationAssertReport(t, otherPreview, want)

	// Повторный анализ после изменения области не наследует удалённые пути.
	var reanalysisLesson, reanalysis resourceusage.Registry
	reanalysisLesson.Register([]string{currentUnused})
	reanalysisLesson.MarkUsed([]string{currentUnused})
	reanalysis.Merge(&reanalysisLesson)
	ruIntegrationAssertReport(t, reanalysis.Snapshot(), resourceusage.Report{
		Total: 1, Used: 1, UsedPaths: []string{currentUnused},
	})
	ruIntegrationAssertReport(t, otherPreview, want)
	ruIntegrationAssertReport(t, previous.Snapshot(), resourceusage.Report{
		Total: 2, Used: 1, Unused: 1,
		UsedPaths: []string{oldUsed}, UnusedPaths: []string{oldUnused},
	})
}

func ruIntegrationAssertReport(t *testing.T, got, want resourceusage.Report) {
	t.Helper()
	if got.Total != want.Total || got.Used != want.Used || got.Unused != want.Unused ||
		!slices.Equal(got.UsedPaths, want.UsedPaths) || !slices.Equal(got.UnusedPaths, want.UnusedPaths) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}
