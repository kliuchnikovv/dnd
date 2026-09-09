package cyberpunk

import (
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

// seqDice — фиктивная кость: отдаёт заранее записанную последовательность
// значений. Первый Roll(1,10) — первый элемент, и т.д.
type seqDice struct{ vals []int }

func (d *seqDice) Roll(n, sides int) int {
	v := d.vals[0]
	d.vals = d.vals[1:]
	_ = n
	_ = sides
	return v
}

func sheetJSON(t *testing.T, s Sheet) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRegisteredInCoreLookup(t *testing.T) {
	// Импорт пакета должен зарегистрировать «cyberpunk» в core.
	if _, ok := core.LookupRuleset(core.RulesetCyberpunk); !ok {
		t.Fatalf("core.LookupRuleset(cyberpunk) должен быть истинен после импорта")
	}
}

func TestVerbsRegisteredGlobally(t *testing.T) {
	for _, name := range []string{"hack", "jack_in", "netrun"} {
		if _, ok := core.Verbs[core.Verb(name)]; !ok {
			t.Errorf("глагол %q не в core.Verbs", name)
		}
	}
}

func TestResolveCheckFallsThrough(t *testing.T) {
	s := New()
	sh := Sheet{DEX: 5, Skills: map[string]int{"stealth": 4}}
	view := core.SceneView{Sheet: sheetJSON(t, sh), DC: DVDifficult}
	in := core.Intent{Verb: "sneak"}
	d := &seqDice{vals: []int{7}} // roll=7, total=7+5+4=16 → success
	res := s.Resolve(in, view, d)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("class %v", res.Class)
	}
}

func TestResolveHackUsesTech(t *testing.T) {
	s := New()
	sh := Sheet{TECH: 6, Skills: map[string]int{"interface": 5}}
	view := core.SceneView{Sheet: sheetJSON(t, sh), DC: DVProfessional}
	in := core.Intent{Verb: "hack"}
	d := &seqDice{vals: []int{7}} // 7+6+5=18 vs 17 → success
	res := s.Resolve(in, view, d)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("class %v (margin=%d)", res.Class, res.Margin)
	}
	// В логе видна метка навыка interface — доказательство маршрута через TECH+interface.
	found := false
	for _, t := range res.Log.Terms {
		if t.Name == "skill:interface" {
			found = true
		}
	}
	if !found {
		t.Errorf("в терминах бросков нет skill:interface: %+v", res.Log.Terms)
	}
}

func TestResolveAttackEmitsHPDeltaMutation(t *testing.T) {
	// «attack» — общий боевой глагол, в core.Verbs его добавляет adventure-
	// сценарий через ExtraVerbs при NewGame. В юнит-тесте резолвера сценария
	// нет; регистрируем явно, как это делает dnd5e (см. registerVerbs).
	core.Verbs["attack"] = core.VerbDef{Verb: "attack", Class: core.ClassAttack, Rolls: true, Hard: true}
	s := New()
	sh := Sheet{
		REF:     6,
		Skills:  map[string]int{"handgun": 4},
		Weapons: []Weapon{{Name: "Medium Pistol", Skill: "handgun", Damage: "2d6"}},
	}
	view := core.SceneView{Sheet: sheetJSON(t, sh), DC: DVDifficult, TargetAC: 4}
	in := core.Intent{Verb: "attack", Args: core.Args{Target: "e_thug"}}
	// d10=7 → total 17 vs 15 hit; damage d(2d6)=8; SP=4 → 4 в HP.
	d := &seqDice{vals: []int{7, 8}}
	res := s.Resolve(in, view, d)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("class %v", res.Class)
	}
	if res.Damage != 4 {
		t.Fatalf("damage %d, want 4", res.Damage)
	}
	if len(res.Mutations) != 1 || res.Mutations[0].Kind != core.MutHPDelta {
		t.Fatalf("нет мутации HPDelta: %+v", res.Mutations)
	}
	if res.Mutations[0].Amount != -4 || res.Mutations[0].Target != "e_thug" {
		t.Fatalf("мутация неправильная: %+v", res.Mutations[0])
	}
}

func TestResolveMoveZoneIsBypass(t *testing.T) {
	s := New()
	view := core.SceneView{}
	in := core.Intent{Verb: "move_zone"}
	// нет кости — паника означала бы, что глагол пробежал по бросковой ветке.
	res := s.Resolve(in, view, &seqDice{vals: nil})
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("move_zone обязан быть mgr-успехом без броска, got %v", res.Class)
	}
}

func TestResolveVerbWithoutRollBypasses(t *testing.T) {
	s := New()
	view := core.SceneView{}
	in := core.Intent{Verb: "jack_in"}
	res := s.Resolve(in, view, &seqDice{vals: nil})
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("jack_in без броска: got %v", res.Class)
	}
}

func TestParseSheetIsolatedFromDND5e(t *testing.T) {
	// dnd5e-лист (Str/Dex/Con/…) в cyberpunk.ParseSheet даёт нулевые STAT'ы:
	// это доказательство изоляции — лист одного правила не разбирается другим
	// как «свой», а превращается в пустой лист.
	dnd := []byte(`{"str":16,"dex":14,"con":15,"int":10,"wis":12,"cha":8,"max_hp":20,"ac":14}`)
	s, err := ParseSheet(dnd)
	if err != nil {
		t.Fatalf("ParseSheet dnd5e-листа не должен падать: %v", err)
	}
	// Ключи "int" и "dex" совпадают у обоих систем — это ожидаемо и не
	// нарушает изоляции (правило само знает, что делать со своими полями).
	// А вот RED-специфичные STAT'ы (REF, BODY, EMP, MOVE, ...) из dnd5e-листа
	// прийти не могут — доказательство того, что два листа НЕ читаются как
	// один и не пересекаются в нетривиальной части.
	if s.REF != 0 || s.BODY != 0 || s.EMP != 0 || s.MOVE != 0 || s.TECH != 0 {
		t.Fatalf("RED-STAT'ы не должны заводиться из dnd5e-листа: %+v", s)
	}
	// dnd5e-специфичные поля (MaxHP, AC) листа cyberpunk не имеют — просто
	// игнорируются json.Unmarshal (unknown fields).
	if s.SP != 0 {
		t.Fatalf("SP не должен браться из dnd5e (у него нет такого поля): %d", s.SP)
	}
}
