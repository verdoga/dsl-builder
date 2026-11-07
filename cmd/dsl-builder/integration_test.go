package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

func TestIntegrationModuleLifecycle(t *testing.T) {
	binary := integrationTestBuild(t)
	root := t.TempDir()
	cwd := filepath.Join(root, "Рабочая папка 日本語")
	for _, dir := range []string{"Входные JSON/Глубже", "Изменения", "Добавление", "Результаты"} {
		if err := os.MkdirAll(filepath.Join(cwd, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	integrationTestCopy(t, "first", filepath.Join(cwd, "Входные JSON", "Первый.json"))
	integrationTestCopy(t, "second", filepath.Join(cwd, "Входные JSON", "Глубже", "Второй.json"))
	integrationTestCopy(t, "changed", filepath.Join(cwd, "Изменения", "Первый.json"))
	integrationTestCopy(t, "added", filepath.Join(cwd, "Добавление", "Третий.json"))
	archive := filepath.Join(cwd, "Результаты", "Учебный модуль.zip")
	before := integrationTestTree(t, root, "")
	binBefore := integrationTestTree(t, filepath.Dir(binary), "")

	create := func() *integrationTestProcess {
		p := integrationTestStart(t, binary, cwd, "--create", "ignored.zip")
		p.choose("Создать новый модуль")
		p.value(`"Входные JSON"`, false)
		p.value("Курс 日本語", false)
		p.value("", true)
		p.choose("Выбрать или изменить каталог и имя архива")
		p.value(`"Результаты"`, false)
		p.value("Учебный модуль.zip", false)
		p.choose("Перейти к подтверждению записи модуля")
		return p
	}
	t.Log("EOF at final create confirmation must leave sources and destination untouched")
	p := create()
	p.until("Ваш выбор:")
	p.finish()
	integrationTestSameTree(t, before, integrationTestTree(t, root, ""))

	t.Log("Create through actual binary and read resulting ZIP through moduleio.Read")
	p = create()
	p.choose("Да")
	screen := p.choose("Выход")
	if !strings.Contains(screen, archive) || !strings.Contains(screen, "Модуль успешно сохранён") {
		t.Fatalf("missing actual result path/success: %s", screen)
	}
	transcript := p.finish()
	if !strings.Contains(transcript, "DSL-Builder v8.0.0-Зима") || !strings.Contains(transcript, "аргументы не используются") {
		t.Fatal("linker version or ignored-arguments notice lost in full process")
	}
	created := integrationTestModule(t, archive, 2)
	first, second := integrationTestFile(t, created, "A801"), integrationTestFile(t, created, "B802")
	if first.Lesson().Document.Version != "1" || second.Lesson().Document.Version != "1" || created.Manifest.Course.Title != "Курс 日本語" {
		t.Fatal("created module metadata mismatch")
	}
	oldSecond := second.OriginalBytes()
	oldArchive := integrationTestRead(t, archive)

	t.Log("Update: decide three fields separately, explicitly prepare revision, then save")
	p = integrationTestStart(t, binary, cwd)
	integrationTestUpdateStart(p)
	seen := make(map[string]bool)
	for range 3 {
		p.choose("Рассмотреть изменение отдельного поля")
		proposal := p.until("Ваш выбор:")
		field := ""
		for _, name := range []string{"document.title", "document.subtitle", "page.title"} {
			if strings.Contains(proposal, name) {
				field = name
			}
		}
		if field == "" || seen[field] {
			t.Fatalf("missing or repeated separate field proposal: %s", proposal)
		}
		seen[field] = true
		answer := "Нет"
		if field == "document.subtitle" {
			answer = "Да"
		}
		p.selectLabel(proposal, answer)
	}
	if !bytes.Equal(oldArchive, integrationTestRead(t, archive)) {
		t.Fatal("update changed ZIP before revision/save consent")
	}
	p.choose("Перейти к итоговому просмотру изменений")
	p.choose("Перейти к подтверждению записи модуля")
	p.choose("Да")
	p.choose("Да")
	p.choose("Выход")
	p.finish()
	updated := integrationTestModule(t, archive, 2)
	changed := integrationTestFile(t, updated, "A801").Lesson()
	if changed.Document.Version != "2" || changed.Document.Title != "Первый урок" || changed.Document.Subtitle == nil || *changed.Document.Subtitle != "Новый подзаголовок" {
		t.Fatalf("update decisions/revision lost: %+v", changed.Document)
	}
	if len(changed.Document.Pages) != 1 || changed.Document.Pages[0].Title == nil || *changed.Document.Pages[0].Title != "Страница" || !bytes.Contains(integrationTestFile(t, updated, "A801").OriginalBytes(), []byte("Исправленный текст")) {
		t.Fatal("accepted content or rejected page title lost")
	}
	if !bytes.Equal(oldSecond, integrationTestFile(t, updated, "B802").OriginalBytes()) {
		t.Fatal("update changed bytes of another lesson")
	}

	t.Log("NoChanges after rejecting remaining title proposals must preserve ZIP bytes and mtime")
	unchanged := integrationTestRead(t, archive)
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	p = integrationTestStart(t, binary, cwd)
	integrationTestUpdateStart(p)
	for range 2 {
		p.choose("Рассмотреть изменение отдельного поля")
		p.choose("Нет")
	}
	p.choose("Перейти к итоговому просмотру изменений")
	screen = p.choose("Выход")
	if !strings.Contains(screen, "Содержательных изменений не обнаружено") {
		t.Fatalf("missing NoChanges terminal result: %s", screen)
	}
	p.finish()
	afterInfo, err := os.Stat(archive)
	if err != nil || !info.ModTime().Equal(afterInfo.ModTime()) || !bytes.Equal(unchanged, integrationTestRead(t, archive)) {
		t.Fatal("NoChanges rewrote archive")
	}

	t.Log("Add a new ID through binary; preserve every existing lesson's exact uncompressed bytes")
	p = integrationTestStart(t, binary, cwd)
	p.choose("Добавить уроки в существующий модуль")
	p.value(`"Результаты/Учебный модуль.zip"`, false)
	p.value(`"Добавление/Третий.json"`, false)
	p.choose("Перейти к подтверждению записи модуля")
	p.choose("Да")
	p.choose("Да")
	p.choose("Выход")
	p.finish()
	added := integrationTestModule(t, archive, 3)
	for _, previous := range updated.Files {
		current := integrationTestFile(t, added, previous.Lesson().Document.ID)
		if previous.FileName() != current.FileName() || !bytes.Equal(previous.OriginalBytes(), current.OriginalBytes()) {
			t.Fatalf("add changed old lesson %s", previous.FileName())
		}
	}
	if integrationTestFile(t, added, "C803").Lesson().Document.Version != "1" {
		t.Fatal("new lesson has wrong initial revision")
	}
	integrationTestSameTree(t, before, integrationTestTree(t, root, archive))
	integrationTestSameTree(t, binBefore, integrationTestTree(t, filepath.Dir(binary), ""))
}

func integrationTestUpdateStart(p *integrationTestProcess) {
	p.t.Helper()
	p.choose("Обновить существующий модуль")
	p.value(`"Результаты/Учебный модуль.zip"`, false)
	p.value(`"Изменения/Первый.json"`, false)
}

func integrationTestModule(t *testing.T, path string, count int) *module.Module {
	t.Helper()
	value, issues := moduleio.Read(path)
	if len(issues) != 0 || value == nil {
		t.Fatalf("read actual archive: %v", issues)
	}
	if len(value.Files) != count {
		t.Fatalf("archive has %d lessons; want %d", len(value.Files), count)
	}
	return value
}

func integrationTestFile(t *testing.T, value *module.Module, id string) *module.File {
	t.Helper()
	for _, file := range value.Files {
		if file.Lesson().Document.ID == id {
			return file
		}
	}
	t.Fatalf("missing actual lesson %s", id)
	return nil
}
