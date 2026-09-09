package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	_ "github.com/kliuchnikovv/dnd/core/scenarios/adventure"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/cyberpunk"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
)

// § этого теста — §3.5 хендоффа: киберпанк-сессия обязана отдать
// Humanity/HP/Heat, а не меры Порога. Реестр view-правил (ADR-0010)
// раскладывает импорт по side-effect'у; здесь мы просто зовём
// view.LookupRuleset(cyberpunk) и убеждаемся, что метки — RED-ные.
func TestCyberpunkViewMetersFromRegistry(t *testing.T) {
	rs, ok := view.LookupRuleset(core.RulesetCyberpunk)
	if !ok {
		t.Fatalf("view.LookupRuleset(cyberpunk) не нашёл — импорт cli не тянет регистрацию?")
	}

	// Строим минимальную игру из готового кейса neon-noir с cyberpunk-листом
	// у актёра. Кейс лежит в cases/neon_noir относительно репо; тест ходит из
	// каталога cli, поэтому путь — относительно него.
	cfg, err := cases.Load(filepath.Join("..", "cases", "neon_noir", "case.json"))
	if err != nil {
		t.Fatalf("load neon-noir: %v", err)
	}
	sys, ok := core.LookupRuleset(core.RulesetCyberpunk)
	if !ok {
		t.Fatalf("core.LookupRuleset(cyberpunk) не нашёл — регистрация rules/cyberpunk не сработала")
	}
	cfg.Rules = sys
	cfg.Dice = dice.NewSource(1).Stream("resolve")

	// Прикручиваем cyberpunk-лист к актёру. cases не заполняет лист — это
	// задача сервер-уровневого CharacterStore (или ручной подстановки здесь).
	sh := cyberpunk.Sheet{
		BODY: 6, WILL: 6, EMP: 7, REF: 6, TECH: 5,
		Skills:    map[string]int{"handgun": 4, "interface": 5, "stealth": 3},
		Cyberware: []cyberpunk.Cyberware{{Name: "Neural Link", HL: 2}},
		SP:        11,
		Weapons:   []cyberpunk.Weapon{{Name: "Medium Pistol", Skill: "handgun", Damage: "2d6"}},
	}
	raw, _ := json.Marshal(sh)
	cfg.DB.SaveCharacter(&store.Character{ID: cfg.Actor, Sheet: raw, Ruleset: string(core.RulesetCyberpunk)})

	g := core.NewGame(*cfg)
	meters := rs.Meters(g)

	kinds := map[string]bool{}
	for _, m := range meters {
		kinds[m.Kind] = true
	}
	// Humanity, HP, Heat — сигнатурные меры RED (Heat — house-rule, часы
	// заведены в case.json). Их наличие — и есть доказательство расширения
	// view-оси новым набором мер.
	for _, want := range []string{"humanity", "hp", "heat"} {
		if !kinds[want] {
			t.Errorf("нет меры %q в meters: %+v", want, meters)
		}
	}
	// Регресс изоляции: ни одной меры threshold — «harm»/«grit»/«clock» —
	// быть не должно (clock мы тоже не отдаём под своим именем, «heat» —
	// собственная метка).
	for _, forbidden := range []string{"harm", "grit"} {
		if kinds[forbidden] {
			t.Errorf("cyberpunk-view не должен светить меру %q Порога", forbidden)
		}
	}
}

// Регресс §3.2 хендоффа: core.LookupRuleset(cyberpunk) истинна после импорта.
func TestCoreLookupRulesetCyberpunk(t *testing.T) {
	if _, ok := core.LookupRuleset(core.RulesetCyberpunk); !ok {
		t.Fatalf("core.LookupRuleset(cyberpunk) обязан быть истинен: импорт rules/cyberpunk?")
	}
}

// Регресс §3.3 хендоффа: киберпанк-глаголы попадают в core.Verbs.
func TestCyberpunkVerbsInCoreRegistry(t *testing.T) {
	for _, name := range []string{"hack", "jack_in", "netrun"} {
		if _, ok := core.Verbs[core.Verb(name)]; !ok {
			t.Errorf("глагол %q не в core.Verbs после импорта rules/cyberpunk", name)
		}
	}
}
