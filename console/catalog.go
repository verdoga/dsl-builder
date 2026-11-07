package console

import (
	"errors"
	"io"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

// editCourse читает обязательное название и вызывает ProposeCourseTitle
// с snapshot.Token; кандидат передаётся reviewEdit. Отмена не вызывает app.
func (d *dialog) editCourse(snapshot app.Snapshot) error {
	title, accepted, err := d.readValue("Название курса:", false)
	if err != nil {
		d.operation.Cancel()
		return err
	}
	if !accepted {
		return nil
	}
	edit, issues := d.operation.ProposeCourseTitle(snapshot.Token, title)
	return d.reviewEdit(snapshot, edit, issues)
}

// editUnit явно поясняет «Enter — убрать юнит» и вызывает ProposeUnitTitle
// только после завершённого ввода; применение решает отдельный reviewEdit.
func (d *dialog) editUnit(snapshot app.Snapshot) error {
	title, accepted, err := d.readValue("Название юнита (Enter — убрать юнит):", true)
	if err != nil {
		d.operation.Cancel()
		return err
	}
	if !accepted {
		return nil
	}
	edit, issues := d.operation.ProposeUnitTitle(snapshot.Token, title)
	return d.reviewEdit(snapshot, edit, issues)
}

// createSection читает непустое исходное имя и вызывает ProposeSection.
// Нормализованное имя и изменения показывает только из полученного EditPreview.
func (d *dialog) createSection(snapshot app.Snapshot) error {
	title, accepted, err := d.readValue("Название новой секции:", false)
	if err != nil {
		d.operation.Cancel()
		return err
	}
	if !accepted {
		return nil
	}
	edit, issues := d.operation.ProposeSection(snapshot.Token, title)
	return d.reviewEdit(snapshot, edit, issues)
}

// moveLesson выбирает файл через chooseLesson и назначение через chooseSection,
// затем вызывает ProposeMove(snapshot.Token, fileName, section). Все уроки,
// включая старые/новые и нераспределённые, доступны для индивидуального переноса.
// nil section — явный пункт «Без секции», а не отмена. Отмена второго вопроса
// отбрасывает первый выбор; подтверждение переноса выполняет reviewEdit.
func (d *dialog) moveLesson(snapshot app.Snapshot) error {
	fileName, accepted, err := d.chooseLesson(snapshot)
	if err != nil {
		d.operation.Cancel()
		return err
	}
	if !accepted {
		return nil
	}
	section, accepted, err := d.chooseSection(snapshot)
	if err != nil {
		d.operation.Cancel()
		return err
	}
	if !accepted {
		return nil
	}
	edit, issues := d.operation.ProposeMove(snapshot.Token, fileName, section)
	return d.reviewEdit(snapshot, edit, issues)
}

// useFlatPlacement предлагает ProposeFlatPlacement(snapshot.Token) только
// после явного выбора пункта завершения размещения без секций. Уроки
// автоматически не переносит; кандидата подтверждает reviewEdit.
func (d *dialog) useFlatPlacement(snapshot app.Snapshot) error {
	edit, issues := d.operation.ProposeFlatPlacement(snapshot.Token)
	return d.reviewEdit(snapshot, edit, issues)
}

// reviewEdit показывает все issues. nil edit означает отказ app создать
// кандидата и возврат к прежнему превью; не трактует это как частичный успех.
// При ненулевом edit показывает showEdit и спрашивает отдельное 1/2.
// Да вызывает AcceptEdit(edit.Token), Нет — RejectEdit(edit.Token).
// Исправляемые Blockers не скрывают возможность принять рабочий кандидат.
// Ошибка показа приводит к RejectEdit; ошибки показа и отклонения объединяет.
// При EOF/ошибке потока отменяет Operation целиком; до принятия группа
// остаётся локальной. Принятие не считается согласием на запись архива.
func (d *dialog) reviewEdit(snapshot app.Snapshot, edit *app.EditPreview, issues []*diagnostic.Error) error {
	closeFailure := func(err error) error {
		streamFailure := isStreamFailure(err)
		if edit != nil {
			if rejection := joinedIssues(d.operation.RejectEdit(edit.Token)); rejection != nil {
				if err == io.EOF {
					err = streamError(CodeInput, err)
				}
				err = errors.Join(err, rejection)
			}
		}
		if streamFailure {
			d.operation.Cancel()
		}
		return err
	}
	if err := d.showIssues(issues); err != nil {
		return closeFailure(err)
	}
	if edit == nil {
		return nil
	}
	if err := d.showEdit(snapshot, edit); err != nil {
		return closeFailure(err)
	}
	accept, err := d.confirm("Применить показанную правку к модели модуля?")
	if err != nil {
		return closeFailure(err)
	}
	var result []*diagnostic.Error
	if accept {
		result = d.operation.AcceptEdit(edit.Token)
	} else {
		result = d.operation.RejectEdit(edit.Token)
	}
	err = d.showIssues(result)
	if isStreamFailure(err) {
		d.operation.Cancel()
	}
	return err
}
