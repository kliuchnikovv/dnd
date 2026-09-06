package threshold

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

// Обстановка учитывается преимуществом/помехой (2d20 бери больше/меньше), НЕ
// сдвигом порога (D&D SRD: Ability Scores; прото §6). DC — от задачи, помехи
// среды — через vantage. Здесь закрепляем и источник vantage, и то, что порог
// он не трогает.

func TestVantageFromEnvironment(t *testing.T) {
	cases := []struct {
		name string
		view core.SceneView
		want int
	}{
		{"нейтрально", core.SceneView{}, 0},
		{"открытая цель — преимущество", core.SceneView{Exposed: true}, +1},
		{"сопротивление — помеха", core.SceneView{Opposed: true}, -1},
		{"темнота — помеха", core.SceneView{NodeTags: []string{"dark"}}, -1},
		{"инструмент отменяет помеху среды", core.SceneView{NodeTags: []string{"dark"}, Tools: []string{"фонарь"}}, 0},
		{"преимущество и помеха гасятся", core.SceneView{Exposed: true, Opposed: true}, 0},
		{"помеха не стакается — всё та же одна", core.SceneView{Opposed: true, NodeTags: []string{"dark"}}, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Vantage(core.Intent{Verb: "examine"}, c.view); got != c.want {
				t.Errorf("Vantage = %d, ожидалось %d", got, c.want)
			}
		})
	}
}

// Пассивное внимание (D&D passive check; прото §5): 10 + модификатор внимания
// (edge), +5 за преимущество обстановки, −5 за помеху. Кости нет — сравнивается
// с порогом тира напрямую.
func TestPassiveScoreIsTenPlusEdgePlusVantage(t *testing.T) {
	edge2 := []byte(`{"attrs":{"edge":2}}`)
	cases := []struct {
		name string
		view core.SceneView
		want int
	}{
		{"ровно: 10+edge", core.SceneView{Sheet: edge2}, 12},
		{"помеха среды −5", core.SceneView{Sheet: edge2, NodeTags: []string{"dark"}}, 7},
		{"преимущество +5", core.SceneView{Sheet: edge2, Exposed: true}, 17},
		{"инструмент снимает помеху среды", core.SceneView{Sheet: edge2, NodeTags: []string{"dark"}, Tools: []string{"фонарь"}}, 12},
		{"без листа — базовые 10", core.SceneView{}, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := New().PassiveScore(c.view); got != c.want {
				t.Errorf("PassiveScore = %d, ожидалось %d", got, c.want)
			}
		})
	}
}

// Преимущество кидает 2d20 и берёт БОЛЬШИЙ.
func TestAdvantageTakesHigherOf2d20(t *testing.T) {
	view := core.SceneView{Exposed: true, Sheet: []byte(`{"attrs":{"mind":0}}`)}
	// Fixed(1, 20): помеха взяла бы 1, преимущество — 20. Порог medium 15.
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(1, 20))
	if got.Log.Die != 20 {
		t.Fatalf("преимущество взяло d20=%d, ожидалось 20 (больший)", got.Log.Die)
	}
}

// Помеха кидает 2d20 и берёт МЕНЬШИЙ.
func TestDisadvantageTakesLowerOf2d20(t *testing.T) {
	view := core.SceneView{Opposed: true, Sheet: []byte(`{"attrs":{"mind":0}}`)}
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(20, 1))
	if got.Log.Die != 1 {
		t.Fatalf("помеха взяла d20=%d, ожидалось 1 (меньший)", got.Log.Die)
	}
}

// Без преимущества/помехи — один d20 (второй не тратится).
func TestNoVantageRollsSingleD20(t *testing.T) {
	view := core.SceneView{Sheet: []byte(`{"attrs":{"mind":0}}`)}
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(7, 20))
	if got.Log.Die != 7 {
		t.Fatalf("без vantage взяли d20=%d, ожидалось 7 (первый и единственный)", got.Log.Die)
	}
}

// Обстановка НЕ двигает порог — только vantage. Порог остаётся medium(15).
func TestEnvironmentDoesNotShiftThreshold(t *testing.T) {
	sheet := []byte(`{"attrs":{"mind":0}}`)
	for _, v := range []core.SceneView{
		{Opposed: true, Sheet: sheet},
		{Exposed: true, Sheet: sheet},
		{NodeTags: []string{"dark"}, Sheet: sheet},
	} {
		got := New().Resolve(core.Intent{Verb: "question"}, v, dice.Fixed(10))
		if got.Log.Threshold != ThresholdNormal {
			t.Errorf("обстановка сдвинула порог до %d, ожидался %d (среда идёт в vantage, не в DC)",
				got.Log.Threshold, ThresholdNormal)
		}
	}
}

// Авторский гейт по-прежнему задаёт порог напрямую (позиция его не перебивает).
func TestAuthorGateStillSetsThreshold(t *testing.T) {
	sheet := []byte(`{"attrs":{"mind":0}}`)
	got := New().Resolve(core.Intent{Verb: "question"},
		core.SceneView{GateThreshold: "hard", Opposed: true, Sheet: sheet}, dice.Fixed(10))
	if got.Log.Threshold != ThresholdHard {
		t.Errorf("авторский гейт hard дал порог %d, ожидался %d", got.Log.Threshold, ThresholdHard)
	}
}

// Нат-1 и нат-20 на проверках характеристик НЕ особые (RAW; прото §10): класс
// определяется только маржой. Крит — по марже ≥+5, не по числу 20.
func TestNoNaturalDieSpecialOnAbilityChecks(t *testing.T) {
	// mind 0, порог medium 15. d20=20 → маржа +5 → крит ПО МАРЖЕ (а не по «20»).
	// Проверяем случай, где число 20, но без спец-правила класс был бы тем же.
	// Ключевой кейс: d20=1 при высоком атрибуте не должен опускать успех.
	view := core.SceneView{Sheet: []byte(`{"attrs":{"mind":18}}`)} // +18: даже d20=1 → 19 vs 15 = успех
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(1))
	if got.Class != core.OutcomeSuccess {
		t.Errorf("нат-1 опустил класс до %v, а по марже (+4) должен быть УСПЕХ (нат-1 не особый)", got.Class)
	}
	if got.Margin != 4 {
		t.Errorf("маржа %d, ожидалась +4 (1+18-15)", got.Margin)
	}
	// d20=20 при низком атрибуте: 20-15=+5 → крит ПО МАРЖЕ; уберём атрибут, чтобы
	// показать, что «крит» — от маржи, а не от числа 20.
	low := core.SceneView{Sheet: []byte(`{"attrs":{"mind":-1}}`)} // 20-1-15 = +4 → успех, НЕ крит
	g2 := New().Resolve(core.Intent{Verb: "question"}, low, dice.Fixed(20))
	if g2.Class != core.OutcomeSuccess {
		t.Errorf("нат-20 при марже +4 дал %v, ожидался УСПЕХ (число 20 не поднимает класс)", g2.Class)
	}
}
