package console

import (
	"strconv"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
)

func TestSelectSectionIncludesEmptySectionsAndCopiesExactTitle(t *testing.T) {
	for index, title := range []string{"Я — первая", "Пустая рабочая", "А — вторая"} {
		t.Run(title, func(t *testing.T) {
			snapshot := selectTestSnapshot()
			before := selectTestBytes(t, snapshot)
			d, output := selectTestDialog(strconv.Itoa(index+1) + "\n")
			got, accepted, err := d.chooseSection(snapshot)
			if err != nil || !accepted || got == nil || *got != title {
				t.Fatalf("chooseSection = (%v, %v, %v), нужно %q", got, accepted, err, title)
			}
			selectTestOrder(t, output.String(), "Я — первая", "Пустая рабочая", "А — вторая", "Без секции")
			selectTestUnchanged(t, before, snapshot)
			*got = "Правка возвращённой строки"
			selectTestUnchanged(t, before, snapshot)
		})
	}
}

func TestSelectSectionDistinguishesExplicitNoSectionFromCancel(t *testing.T) {
	for _, count := range []int{0, 3} {
		for _, cancel := range []bool{false, true} {
			t.Run(strconv.Itoa(count)+"/cancel="+strconv.FormatBool(cancel), func(t *testing.T) {
				snapshot := selectTestSnapshot()
				snapshot.Catalog.Sections = snapshot.Catalog.Sections[:count]
				if count == 0 {
					snapshot.Catalog.Sectioned = false
					for index := range snapshot.Catalog.Items {
						snapshot.Catalog.Items[index].Section = nil
					}
				}
				choice := count + 1
				if cancel {
					choice++
				}
				before := selectTestBytes(t, snapshot)
				d, output := selectTestDialog(strconv.Itoa(choice) + "\n")
				got, accepted, err := d.chooseSection(snapshot)
				if err != nil || accepted == cancel || (!cancel && got != nil) {
					t.Fatalf("chooseSection = (%v, %v, %v), cancel=%v", got, accepted, err, cancel)
				}
				if !strings.Contains(output.String(), "Без секции") {
					t.Fatal("нет явного пункта без секции")
				}
				selectTestUnchanged(t, before, snapshot)
			})
		}
	}
}

func TestSelectSectionPreservesUnicodeAndEscapesControls(t *testing.T) {
	snapshot := selectTestSnapshot()
	title := "  世界🙂 Кириллица " + strings.Repeat("секция", 12000) + " КОНЕЦ  "
	snapshot.Catalog.Sections = []catalog.SectionOrder{{Title: title}, {Title: "Начало\nПОДМЕНА\tКонец\x1b[2J"}}
	for index := range snapshot.Catalog.Items {
		snapshot.Catalog.Items[index].Section = selectTestString(title)
	}
	before := selectTestBytes(t, snapshot)
	d, output := selectTestDialog("1\n")
	got, accepted, err := d.chooseSection(snapshot)
	if err != nil || !accepted || got == nil || *got != title || !strings.Contains(output.String(), title) {
		t.Fatal("выбор/показ изменил длинное точное название Unicode")
	}
	for _, forbidden := range []string{"\nПОДМЕНА", "\t", "\x1b"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("неэкранированный внешний control %q", forbidden)
		}
	}
	for _, preserved := range []string{"Начало", "ПОДМЕНА", "Конец", "2J", "\\"} {
		if !strings.Contains(output.String(), preserved) {
			t.Fatalf("отображение потеряло %q", preserved)
		}
	}
	selectTestUnchanged(t, before, snapshot)
}

func TestSelectGroupReturnsExactUnicodeTitleAndEscapesControls(t *testing.T) {
	snapshot := selectTestSnapshot()
	title := "Кириллица 世界🙂\x1b[2J"
	snapshot.Catalog.Sections[0].Title = title
	snapshot.Catalog.Items[2].Section = selectTestString(title)
	before := selectTestBytes(t, snapshot)
	d, output := selectTestDialog("1\n")
	got, accepted, err := d.chooseGroup(snapshot)
	if err != nil || !accepted || got == nil || *got != title {
		t.Fatalf("chooseGroup изменил точное имя: (%v, %v, %v)", got, accepted, err)
	}
	if strings.Contains(output.String(), "\x1b") || !strings.Contains(output.String(), "Кириллица 世界🙂") || !strings.Contains(output.String(), "2J") || !strings.Contains(output.String(), "\\") {
		t.Fatal("внешний control не экранирован либо потерян Unicode")
	}
	selectTestUnchanged(t, before, snapshot)
}

func TestSelectGroupOnlyNonemptyGroupsInSectionsOrderThenUnplaced(t *testing.T) {
	for index, expected := range []*string{selectTestString("Я — первая"), selectTestString("А — вторая"), nil} {
		t.Run(strconv.Itoa(index+1), func(t *testing.T) {
			snapshot := selectTestSnapshot()
			before := selectTestBytes(t, snapshot)
			d, output := selectTestDialog(strconv.Itoa(index+1) + "\n")
			got, accepted, err := d.chooseGroup(snapshot)
			if err != nil || !accepted || (got == nil) != (expected == nil) || (expected != nil && *got != *expected) {
				t.Fatalf("chooseGroup = (%v, %v, %v), нужен %v", got, accepted, err, expected)
			}
			selectTestOrder(t, output.String(), "Я — первая", "А — вторая", "Не распределены")
			if strings.Contains(output.String(), "Пустая рабочая") || strings.Contains(output.String(), "Без секций") {
				t.Fatal("меню содержит пустую группу либо неверное имя нераспределённых")
			}
			selectTestUnchanged(t, before, snapshot)
		})
	}
}

func TestSelectGroupFlatLabelAndAbsentUnplacedGroup(t *testing.T) {
	t.Run("плоская группа", func(t *testing.T) {
		snapshot := selectTestSnapshot()
		snapshot.Catalog.Sectioned = false
		snapshot.Catalog.Sections = nil
		for index := range snapshot.Catalog.Items {
			snapshot.Catalog.Items[index].Section = nil
			snapshot.Catalog.Items[index].Entry.Order = nil
		}
		before := selectTestBytes(t, snapshot)
		d, output := selectTestDialog("1\n")
		got, accepted, err := d.chooseGroup(snapshot)
		if err != nil || !accepted || got != nil || !strings.Contains(output.String(), "Без секций") || strings.Contains(output.String(), "Не распределены") {
			t.Fatalf("плоская группа = (%v, %v, %v), вывод %q", got, accepted, err, output.String())
		}
		selectTestUnchanged(t, before, snapshot)
	})
	t.Run("все распределены", func(t *testing.T) {
		snapshot := selectTestSnapshot()
		snapshot.Catalog.Items[1].Section = selectTestString("А — вторая")
		before := selectTestBytes(t, snapshot)
		d, output := selectTestDialog("3\n")
		_, accepted, err := d.chooseGroup(snapshot)
		if err != nil || accepted {
			t.Fatalf("после двух непустых групп нужна отмена: (%v, %v)", accepted, err)
		}
		if strings.Contains(output.String(), "Не распределены") || strings.Contains(output.String(), "Без секций") {
			t.Fatal("предложена отсутствующая группа без секции")
		}
		selectTestUnchanged(t, before, snapshot)
	})
}

func TestSelectGroupDoesNotInterpretWeightsOrReorderSnapshot(t *testing.T) {
	snapshot := selectTestSnapshot()
	for index := range snapshot.Catalog.Items {
		snapshot.Catalog.Items[index].Entry.Order = nil
	}
	snapshot.Catalog.Sections[0].Weight = selectTestString(strings.Repeat("9", 200))
	snapshot.Catalog.Sections[2].Weight = selectTestString("0001")
	before := selectTestBytes(t, snapshot)
	d, output := selectTestDialog("1\n")
	got, accepted, err := d.chooseGroup(snapshot)
	if err != nil || !accepted || got == nil || *got != "Я — первая" {
		t.Fatalf("выбрана не первая группа Sections: (%v, %v, %v)", got, accepted, err)
	}
	selectTestOrder(t, output.String(), "Я — первая", "А — вторая", "Не распределены")
	selectTestUnchanged(t, before, snapshot)
}
