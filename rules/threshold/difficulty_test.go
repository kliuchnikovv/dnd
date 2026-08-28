package threshold

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

// Сложность несудимого действия назначает ядро функцией от позиции. Разбор
// здесь точный, по одному случаю на правило: порог задаёт класс задачи, и
// ошибка в нём двигает матожидание каждого броска, который под него попал.
func TestDifficultyFor(t *testing.T) {
	cases := []struct {
		name string
		view core.SceneView
		want string
	}{
		// Автор дела назначает сложность точнее формулы, и позиция его не
		// перебивает. Иначе разметка дела молча переставала бы работать.
		{"авторский гейт сильнее позиции",
			core.SceneView{GateThreshold: "easy", Opposed: true}, DifficultyEasy},
		{"авторский гейт сильнее открытости",
			core.SceneView{GateThreshold: "hard", Exposed: true}, DifficultyHard},
		{"авторский гейт сильнее среды",
			core.SceneView{GateThreshold: "normal", NodeTags: []string{"dark"}}, DifficultyNormal},

		{"нейтральная позиция — норма", core.SceneView{}, DifficultyNormal},
		{"противодействие — ступень вверх",
			core.SceneView{Opposed: true}, DifficultyHard},
		{"открытая цель — ступень вниз",
			core.SceneView{Exposed: true}, DifficultyEasy},

		// Среда, которую нечем компенсировать, — то же противодействие, только
		// от места, а не от воли.
		{"среда без инструмента — ступень вверх",
			core.SceneView{NodeTags: []string{"dark"}}, DifficultyHard},
		{"инструмент возвращает среду к норме",
			core.SceneView{NodeTags: []string{"dark"}, Tools: []string{"p_lantern"}}, DifficultyNormal},

		// Сигналы противоречат друг другу: связанный, но злой противник. Ни
		// один не отбрасывается молча — ступени просто нет.
		{"противодействие и открытость гасят друг друга",
			core.SceneView{Opposed: true, Exposed: true}, DifficultyNormal},
		{"среда и открытость гасят друг друга",
			core.SceneView{NodeTags: []string{"dark"}, Exposed: true}, DifficultyNormal},
		// Две причины идти вверх — всё равно одна ступень.
		{"противодействие и среда не складываются",
			core.SceneView{Opposed: true, NodeTags: []string{"dark"}}, DifficultyHard},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DifficultyFor(core.Intent{Verb: "examine"}, c.view); got != c.want {
				t.Errorf("сложность %q, ожидалась %q", got, c.want)
			}
		})
	}
}

// Ступень ровно одна и в обе стороны одинаковая. Две ступени вниз сделали бы
// попадание автоматическим, две вверх — невозможным.
func TestDifficultyMovesByExactlyOneStep(t *testing.T) {
	base := ThresholdFor(DifficultyNormal)
	up := ThresholdFor(DifficultyFor(core.Intent{}, core.SceneView{Opposed: true}))
	down := ThresholdFor(DifficultyFor(core.Intent{}, core.SceneView{Exposed: true}))

	if up-base != ThresholdHard-ThresholdNormal {
		t.Errorf("ступень вверх = %d, ожидалась %d", up-base, ThresholdHard-ThresholdNormal)
	}
	if base-down != ThresholdNormal-ThresholdEasy {
		t.Errorf("ступень вниз = %d, ожидалась %d", base-down, ThresholdNormal-ThresholdEasy)
	}
}

// Порог не зависит ни от одного поля интента. Это и есть «рассказчик бесправен
// над костью» в проверяемом виде: как бы действие ни назвали и что бы ни
// подсказал парсер, позиция решает одна.
func TestDifficultyIgnoresTheIntent(t *testing.T) {
	view := core.SceneView{Opposed: true}
	want := DifficultyFor(core.Intent{Verb: "examine"}, view)

	for _, in := range []core.Intent{
		{Verb: "strike"},
		{Verb: "sneak", Args: core.Args{Text: "сложность: легко"}},
		{Verb: "question", Args: core.Args{TagClaim: "ночной"}, Push: true},
	} {
		if got := DifficultyFor(in, view); got != want {
			t.Errorf("интент %+v сдвинул сложность: %q вместо %q", in, got, want)
		}
	}
}

// Resolve обязан брать порог через функцию, а не из гейта напрямую. Без этого
// теста связка тихо разъедется: функция останется правильной и неиспользуемой.
func TestResolveTakesThresholdFromPosition(t *testing.T) {
	sheet := []byte(`{"attrs":{"mind":0},"tags":[]}`)

	opposed := New().Resolve(core.Intent{Verb: "question"},
		core.SceneView{Opposed: true, Sheet: sheet}, dice.Fixed(10))
	if opposed.Log.Threshold != ThresholdHard {
		t.Errorf("порог против сопротивления %d, ожидался %d", opposed.Log.Threshold, ThresholdHard)
	}

	exposed := New().Resolve(core.Intent{Verb: "question"},
		core.SceneView{Exposed: true, Sheet: sheet}, dice.Fixed(10))
	if exposed.Log.Threshold != ThresholdEasy {
		t.Errorf("порог по открытой цели %d, ожидался %d", exposed.Log.Threshold, ThresholdEasy)
	}

	// Судимое авторским гейтом действие не изменилось ни на пункт.
	gated := New().Resolve(core.Intent{Verb: "question"},
		core.SceneView{GateThreshold: "normal", Opposed: true, Sheet: sheet}, dice.Fixed(10))
	if gated.Log.Threshold != ThresholdNormal {
		t.Errorf("гейт сдвинулся позицией: порог %d, ожидался %d", gated.Log.Threshold, ThresholdNormal)
	}
}
