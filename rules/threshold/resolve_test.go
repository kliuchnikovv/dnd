package threshold

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

func sheetRaw(t *testing.T) core.SceneView {
	t.Helper()
	return core.SceneView{
		GateThreshold: "normal",
		Sheet:         []byte(`{"attrs":{"body":0,"edge":0,"mind":4,"will":0},"tags":[]}`),
	}
}

func TestMarginBoundsPickClass(t *testing.T) {
	// Порог 14, mind +4: маржа = d20 - 10.
	cases := []struct {
		die    int
		want   core.Outcome
		margin int
	}{
		{15, core.OutcomeCrit, 5},
		{14, core.OutcomeSuccess, 4},
		{10, core.OutcomeSuccess, 0},
		{9, core.OutcomePartial, -1},
		{6, core.OutcomePartial, -4},
		{5, core.OutcomeFail, -5},
		{2, core.OutcomeFail, -8},
	}
	for _, c := range cases {
		got := New().Resolve(core.Intent{Verb: "question"}, sheetRaw(t), dice.Fixed(c.die))
		if got.Class != c.want {
			t.Errorf("d20=%d: класс %v, ожидался %v", c.die, got.Class, c.want)
		}
		if got.Margin != c.margin {
			t.Errorf("d20=%d: маржа %d, ожидалась %d", c.die, got.Margin, c.margin)
		}
	}
}

func TestNat20StepsUpAndNat1StepsDown(t *testing.T) {
	view := sheetRaw(t)
	view.GateThreshold = "hard" // порог 18, маржа = d20 - 14

	// d20=20 даёт маржу +6 (крит) и остаётся критом: выше ступени нет.
	if got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(20)); got.Class != core.OutcomeCrit {
		t.Errorf("нат-20: класс %v, ожидался КРИТ", got.Class)
	}
	// d20=1 даёт маржу -13 (провал) и остаётся провалом: ниже ступени нет.
	if got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(1)); got.Class != core.OutcomeFail {
		t.Errorf("нат-1: класс %v, ожидался ПРОВАЛ", got.Class)
	}
}

func TestNat20LiftsSuccessToCrit(t *testing.T) {
	// Порог 18, mind +4: d20=20 даёт маржу +6, уже крит. Нужен случай, где
	// нат-20 действительно поднимает: mind 0, порог 18 -> маржа +2 = успех.
	view := core.SceneView{GateThreshold: "hard",
		Sheet: []byte(`{"attrs":{"mind":0},"tags":[]}`)}
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(20))
	if got.Class != core.OutcomeCrit {
		t.Errorf("нат-20 не поднял УСПЕХ до КРИТА: %v (маржа %d)", got.Class, got.Margin)
	}
}

// grit — единственное место, где игрок принимает решение о риске. Тратится он
// заявкой ДО броска: автоматическое списание на каждом провале решением не
// является, потому что решать нечего.
func TestPushSpendsGritForTwo(t *testing.T) {
	view := sheetRaw(t)
	view.Grit = 3
	plain := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(9))
	pushed := New().Resolve(core.Intent{Verb: "question", Push: true}, view, dice.Fixed(9))

	if pushed.Margin != plain.Margin+2 {
		t.Errorf("push дал %+d к марже, ожидалось +2", pushed.Margin-plain.Margin)
	}
	var spent bool
	for _, m := range pushed.Mutations {
		if m.Kind == core.MutResource && m.Target == "grit" && m.Delta == -1 {
			spent = true
		}
	}
	if !spent {
		t.Error("push сработал, но grit не списался")
	}
}

func TestPushWithoutGritChangesNothing(t *testing.T) {
	view := sheetRaw(t)
	view.Grit = 0
	plain := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(9))
	pushed := New().Resolve(core.Intent{Verb: "question", Push: true}, view, dice.Fixed(9))
	if pushed.Margin != plain.Margin || len(pushed.Mutations) != 0 {
		t.Errorf("push без grit что-то изменил: маржа %d против %d, мутации %v",
			pushed.Margin, plain.Margin, pushed.Mutations)
	}
}

// Без заявки провал остаётся провалом, сколько бы grit ни лежало на листе.
func TestFailStaysFailWithoutPush(t *testing.T) {
	view := sheetRaw(t)
	view.Grit = 3
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(2))
	if got.Class != core.OutcomeFail {
		t.Errorf("провал сконвертирован без заявки: %v", got.Class)
	}
}

func TestNonRollingVerbSucceedsWithoutDice(t *testing.T) {
	got := New().Resolve(core.Intent{Verb: "compare"}, sheetRaw(t), dice.Fixed(1))
	if got.Class != core.OutcomeSuccess {
		t.Errorf("compare дал %v — глагол без броска обязан просто удаваться", got.Class)
	}
	if got.Log.Die != 0 {
		t.Errorf("compare бросил кость: %d", got.Log.Die)
	}
}

func TestLogCarriesNamedTerms(t *testing.T) {
	got := New().Resolve(core.Intent{Verb: "question"}, sheetRaw(t), dice.Fixed(11))
	if got.Log.Die != 11 || got.Log.Threshold != 14 {
		t.Errorf("лог броска неполон: %+v", got.Log)
	}
	names := map[string]int{}
	for _, term := range got.Log.Terms {
		names[term.Name] = term.Value
	}
	if names["атрибут"] != 4 {
		t.Errorf("в логе нет слагаемого «атрибут»: %+v", got.Log.Terms)
	}
}

func TestSystemSatisfiesRuleSystem(t *testing.T) {
	var _ core.RuleSystem = New()
}

// Переход по улице, где никто не мешает, — не бросок. Провал, означающий «ты
// не дошёл до соседнего дома», не несёт ни выбора, ни смысла, зато тикает часы
// и рассинхронизирует всё, что игрок планировал дальше.
func TestUnopposedMoveDoesNotRoll(t *testing.T) {
	view := sheetRaw(t)
	got := New().Resolve(core.Intent{Verb: "move_zone"}, view, dice.Fixed(1))
	if got.Class != core.OutcomeSuccess {
		t.Errorf("безопасный переход дал %v", got.Class)
	}
	if got.Log.Die != 0 {
		t.Errorf("безопасный переход тронул кость: d20=%d", got.Log.Die)
	}
}

// При враждебных в сцене переход снова становится броском: уйти из-под
// наблюдения — это уже действие с ценой.
func TestMoveUnderHostilesStillRolls(t *testing.T) {
	view := sheetRaw(t)
	view.Foes = 1
	got := New().Resolve(core.Intent{Verb: "move_zone"}, view, dice.Fixed(1))
	if got.Log.Die == 0 {
		t.Error("переход при враждебных прошёл без броска")
	}
}
