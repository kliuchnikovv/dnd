package dnd5e

import (
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

// registerVerbs добавляет все глаголы D&D в реестр core.Verbs.
// Task 16 (adventure archetype) будет делать это в Scenario.ExtraVerbs() при
// запуске сценария. Здесь регистрируем явно для санитарного теста.
func registerVerbs() {
	for _, def := range Verbs() {
		core.Verbs[def.Verb] = def
	}
}

func TestSystemResolvesHide(t *testing.T) {
	// Регистрируем dnd5e глаголы в core.Verbs перед резолвом
	registerVerbs()

	s := Sheet{Dex: 16, Prof: 2, Skills: []string{"stealth"}}
	raw, _ := json.Marshal(s)
	sys := New()
	d := dice.NewSource(1).Stream("resolve")
	res := sys.Resolve(
		core.Intent{Verb: "hide"},
		core.SceneView{Sheet: raw},
		d,
	)
	// Проверяем, что мы прошли через resolveCheck, а не вернули тривиальный результат.
	// С seed 1, stealth check не проходит (roll=4, total=9, DC=12),
	// поэтому получаем OutcomeFail с Margin = 3.
	// Это доказывает, что зарегистрированные verbs работают и резолвер вызвал resolveCheck.
	if res.Class != core.OutcomeFail {
		t.Fatalf("ожидали OutcomeFail (0), получили %d", res.Class)
	}
	if res.Margin == 0 {
		t.Fatal("Margin должна быть непустой для реального резолва")
	}
}
