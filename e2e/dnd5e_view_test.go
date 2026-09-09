package e2e

import (
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/dnd5e"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"

	// Регистрирует view-меры dnd5e — тот самый шов, который проверяет
	// приёмка спеки 2: dnd5e-сессия отдаёт HP, а не меры Порога.
	_ "github.com/kliuchnikovv/dnd/rules/dnd5e/view5e"
)

// TestDND5eSessionUsesOwnMetersNotThreshold — приёмка спеки 2/3 (ADR-0010).
// view.Build с view-правилом, поднятым из реестра по имени "dnd5e",
// показывает меры dnd5e: HP актёра. Меры Порога (harm/grit/clock) в
// наборе отсутствуют — dnd5e больше не фолбэкается на чужое правило.
func TestDND5eSessionUsesOwnMetersNotThreshold(t *testing.T) {
	cfg, err := cases.Load("../cases/lighthouse/case.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = dnd5e.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	g := core.NewGame(*withActorCharacter(cfg))

	rs, ok := view.LookupRuleset(core.RulesetDND5e)
	if !ok {
		t.Fatal("view.LookupRuleset(dnd5e) не находит фабрики — импорт view5e не отработал")
	}
	tv := view.Build(g, core.TurnResult{}, nil, rs, cli.Detective{}, "")

	// HP актёра должен присутствовать: лист lighthouse даёт chr_kay 12/12.
	var hp *view.Meter
	for i := range tv.Meters {
		if tv.Meters[i].Kind == "hp" {
			hp = &tv.Meters[i]
			break
		}
	}
	if hp == nil {
		t.Fatalf("HP-меры нет во view: %+v", tv.Meters)
	}
	ent := g.DB.Entities[store.EntityID(g.Actor)]
	if hp.Value != ent.HP || hp.Max == nil || *hp.Max != ent.MaxHP {
		t.Errorf("HP не совпал с сущностью: мера=%+v ent=%+v", hp, ent)
	}

	// Меры Порога — чужой ассортимент. Их присутствие означало бы фолбэк
	// на RefRuleset, ровно тот регресс, который спека 2 закрывает.
	for _, m := range tv.Meters {
		switch m.Kind {
		case "harm", "grit", "clock":
			t.Errorf("dnd5e-сессия несёт меру Порога %q: %+v", m.Kind, m)
		}
	}
}

// TestThresholdSessionUnchanged — регресс-охрана: сессия Порога после
// появления dnd5e-мер выбирает своё правило и продолжает отдавать harm/
// grit/clock, а не hp. Пара «правило по имени сессии» работает в обе
// стороны от реестра.
func TestThresholdSessionUnchanged(t *testing.T) {
	rs, ok := view.LookupRuleset(core.RulesetThreshold)
	if !ok {
		t.Fatal("Порог должен оставаться зарегистрированным")
	}
	if _, isDND := rs.(interface{ isDND5e() }); isDND {
		t.Fatalf("реестр вернул не Порог: %T", rs)
	}
}
