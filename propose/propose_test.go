package propose

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/store"
)

func game() *core.Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	return core.NewGame(core.Config{DB: db, CaseID: "harbour", Start: "n_quay"})
}

func canon(topic, text string) core.Mutation {
	return core.Mutation{Kind: core.MutCanonAmbient, Target: topic, Text: text, Delta: 1}
}

// Роль с полномочием проводит канон: гейт пропускает, ядро применяет.
func TestRoleWithCapabilityPassesCanon(t *testing.T) {
	g := game()
	app, ref := Mutation(g, llm.RoleNarrator, canon("погода", "морось с моря"))
	if ref.Refused() {
		t.Fatalf("нарратор не провёл канон: %q", ref.Reason)
	}
	if app.Text != "морось с моря" {
		t.Errorf("применено %+v", app)
	}
	if got, ok := g.CanonGet("погода"); !ok || got != "морось с моря" {
		t.Errorf("канон не записан: %q (%v)", got, ok)
	}
}

// Роль без полномочия отсекается ДО ядра. Причина — гейтовая: если бы отказ
// пришёл из ядра, полномочия проверялись бы уже только на словах.
func TestRoleWithoutCapabilityIsRefusedAtGate(t *testing.T) {
	g := game()
	for _, role := range []llm.Role{llm.RoleActor, llm.RoleCanonGuard,
		llm.RoleModeration, llm.RoleIntentParser} {
		app, ref := Mutation(g, role, canon("погода", "морось с моря"))
		if ref.Reason != RefusalNotAllowed {
			t.Errorf("роль %q: причина %q, ждали гейтовую", role, ref.Reason)
		}
		if app.Kind != "" {
			t.Errorf("роль %q что-то применила: %+v", role, app)
		}
	}
	if got := g.Canon(); len(got) != 0 {
		t.Errorf("ядро вызвано мимо гейта — канон записан: %+v", got)
	}
}

// Незаявленная роль полномочий не имеет: молчание реестра — это «ничего не
// предлагает», а не «можно всё».
func TestUnknownRoleProposesNothing(t *testing.T) {
	g := game()
	if _, ref := Mutation(g, llm.Role("самозванец"), canon("погода", "морось")); ref.Reason != RefusalNotAllowed {
		t.Errorf("незаявленная роль получила %q", ref.Reason)
	}
}

// Канон вправе предлагать ровно две роли. Список здесь закрыт нарочно: новое
// полномочие в реестре придётся вписать и сюда — и объяснить, зачем.
func TestOnlyTwoRolesMayProposeCanon(t *testing.T) {
	allowed := map[llm.Role]bool{llm.RoleNarrator: true, llm.RoleWorldsmith: true}
	for _, role := range llm.AllRoles() {
		if got := Allowed(role, core.MutCanonAmbient); got != allowed[role] {
			t.Errorf("роль %q: канон разрешён=%v, ждали %v", role, got, allowed[role])
		}
	}
}

// Числовой вид не проводит никто и нигде: в реестре нет полномочия, которое
// бы его покрывало, а за гейтом его отвергло бы и ядро.
func TestNumericKindsPassNoRole(t *testing.T) {
	g := game()
	for _, role := range llm.AllRoles() {
		for _, kind := range []core.MutationKind{core.MutHarm, core.MutResource,
			core.MutClock, core.MutDisposition, core.MutPosition} {
			if Allowed(role, kind) {
				t.Errorf("роль %q вправе предлагать %q", role, kind)
			}
			_, ref := Mutation(g, role, core.Mutation{Kind: kind, Target: "pc", Delta: 1})
			if ref.Reason != RefusalNotAllowed {
				t.Errorf("роль %q, вид %q: причина %q", role, kind, ref.Reason)
			}
		}
	}
}

// Рассказать дорогу вправе тот, кто говорит, — актёр. Это осознанное
// расширение его полномочий: до сих пор Proposes у него был пуст.
func TestActorMayTellThePlace(t *testing.T) {
	g := game() // n_quay, дело harbour
	g.DB.Locations["n_warehouse"] = store.Location{ID: "n_warehouse"}
	g.DB.Locations["n_quay"] = store.Location{ID: "n_quay",
		Adjacent: []store.NodeID{"n_warehouse"}}

	if _, ref := Mutation(g, llm.RoleActor,
		core.Mutation{Kind: core.MutPlaceKnown, Target: "n_warehouse"}); ref.Refused() {
		t.Errorf("актёр не смог рассказать дорогу: %q", ref.Reason)
	}
	if !g.KnowsPlace("n_warehouse") {
		t.Error("место не стало известным")
	}
}

// Место вправе рассказать ровно одна роль. Список закрыт нарочно: судья и
// модерация состояния не касаются вовсе, а Мастер говорит не за персонажа.
func TestOnlyActorMayTellPlaces(t *testing.T) {
	allowed := map[llm.Role]bool{llm.RoleActor: true}
	for _, role := range llm.AllRoles() {
		if got := Allowed(role, core.MutPlaceKnown); got != allowed[role] {
			t.Errorf("роль %q: место разрешено=%v, ждали %v", role, got, allowed[role])
		}
	}
}

// Порядок гейтов виден на MutWorldEvent: куратор проходит капабилити и
// упирается в ядро, нарратор не проходит уже гейт. Пройти обязаны оба.
func TestWorldEventPassesGateAndStopsInCore(t *testing.T) {
	g := game()
	_, ref := Mutation(g, llm.RoleWorldsmith, core.Mutation{Kind: core.MutWorldEvent, Target: "n_quay"})
	if !ref.Refused() {
		t.Fatal("событие мира применилось без обработчика")
	}
	if ref.Reason == RefusalNotAllowed {
		t.Error("куратор не прошёл капабилити-гейт — а он вправе предлагать событие мира")
	}
	if _, ref := Mutation(g, llm.RoleNarrator, core.Mutation{Kind: core.MutWorldEvent}); ref.Reason != RefusalNotAllowed {
		t.Errorf("нарратор провёл событие мира: %q", ref.Reason)
	}
}
