package cli

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/view"
)

// meterByKind — мера заданного вида из набора, для проверок surface.
func meterByKind(ms []view.Meter, kind string) (view.Meter, bool) {
	for _, m := range ms {
		if m.Kind == kind {
			return m, true
		}
	}
	return view.Meter{}, false
}

// TestRulesetHidesHarmAtFullHealth — здоровый персонаж не видит меру ран: её
// нет причины поднимать, и минимализм держится флагом правила, а не глазом
// клиента.
func TestRulesetHidesHarmAtFullHealth(t *testing.T) {
	g := renderGame(t)
	harm, ok := meterByKind(RefRuleset{}.Meters(g), "harm")
	if !ok {
		t.Fatal("меры ран нет в наборе")
	}
	if harm.Surface {
		t.Errorf("на полном здоровье раны всплыли: %+v", harm)
	}
}

// TestRulesetSurfacesHarmWhenWounded — ранение поднимает меру. Это и есть
// surfacing: мера всплывает, когда стала важна сейчас.
func TestRulesetSurfacesHarmWhenWounded(t *testing.T) {
	g := renderGame(t)
	g.DB.Characters[g.Actor].Harm = 1
	harm, _ := meterByKind(RefRuleset{}.Meters(g), "harm")
	if !harm.Surface || harm.Value != 1 {
		t.Errorf("ранение не подняло меру: %+v", harm)
	}
	if harm.Max == nil || *harm.Max != core.HarmMax {
		t.Errorf("у раны нет потолка: %+v", harm)
	}
}

// TestRulesetSurfacesClockNearFull — часы всплывают, когда близки к срабатыванию,
// и молчат, пока далеко.
func TestRulesetSurfacesClockNearFull(t *testing.T) {
	g := renderGame(t)
	fresh, ok := meterByKind(RefRuleset{}.Meters(g), "clock")
	if !ok {
		t.Fatal("часов нет в наборе мер")
	}
	if fresh.Surface {
		t.Errorf("свежие часы всплыли: %+v", fresh)
	}
	// До предпоследнего сегмента: один тик до срабатывания.
	g.C.Tick("c_suspicion", *fresh.Max-1)
	near, _ := meterByKind(RefRuleset{}.Meters(g), "clock")
	if !near.Surface {
		t.Errorf("часы у порога не всплыли: %+v", near)
	}
}

// TestDetectiveObjectiveHiddenByDefault — панель цели скрыта по умолчанию:
// вызывается жестом, а не висит на экране.
func TestDetectiveObjectiveHiddenByDefault(t *testing.T) {
	g := renderGame(t)
	p := Detective{}.Objective(g)
	if p == nil {
		t.Fatal("панель цели пуста")
	}
	if p.Surface {
		t.Errorf("панель всплыла сама: %+v", p)
	}
	if p.Kind != "deduction" {
		t.Errorf("архетип панели не дедукция: %q", p.Kind)
	}
}

// TestDetectiveObjectiveListsKnownFacts — известный факт попадает в панель под
// своими словами, а не идентификатором.
func TestDetectiveObjectiveListsKnownFacts(t *testing.T) {
	g := renderGame(t)
	p := Detective{}.Objective(g)
	want := g.DB.Facts["f_ligature"].Key
	found := false
	for _, s := range p.Sections {
		for _, it := range s.Items {
			if it.ID == "f_ligature" && it.Label == want {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("известный факт f_ligature не в панели под словами %q: %+v", want, p.Sections)
	}
}

// TestDetectiveObjectiveHasAccusationSlots — панель несёт слоты обвинения
// who/how/when/why, подкреплённые собранными фактами.
func TestDetectiveObjectiveHasAccusationSlots(t *testing.T) {
	g := renderGame(t)
	p := Detective{}.Objective(g)
	slots := map[string]view.Slot{}
	for _, s := range p.Sections {
		for _, sl := range s.Slots {
			slots[sl.Name] = sl
		}
	}
	for _, name := range []string{"who", "how", "when", "why"} {
		sl, ok := slots[name]
		if !ok {
			t.Errorf("слота %q нет в панели", name)
			continue
		}
		if len(sl.Options) == 0 {
			t.Errorf("слот %q без опций, хотя факт собран", name)
		}
	}
}
