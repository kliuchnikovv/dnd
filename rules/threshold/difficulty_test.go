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

		// Обстановка порог БОЛЬШЕ не двигает — она идёт в vantage (2d20), не в DC
		// (см. TestVantageFromEnvironment). Любая позиция без авторского гейта =
		// medium.
		{"сопротивление порог не двигает (→ vantage)",
			core.SceneView{Opposed: true}, DifficultyNormal},
		{"открытость порог не двигает (→ vantage)",
			core.SceneView{Exposed: true}, DifficultyNormal},
		{"темнота порог не двигает (→ vantage)",
			core.SceneView{NodeTags: []string{"dark"}}, DifficultyNormal},
		{"открытость+темнота — всё равно medium (в DC ничего)",
			core.SceneView{NodeTags: []string{"dark"}, Exposed: true}, DifficultyNormal},
		{"сопротивление в темноте — всё равно medium",
			core.SceneView{Opposed: true, NodeTags: []string{"dark"}}, DifficultyNormal},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DifficultyFor(core.Intent{Verb: "examine"}, c.view); got != c.want {
				t.Errorf("сложность %q, ожидалась %q", got, c.want)
			}
		})
	}
}

// Авторский гейт ложится на каноничную шкалу DC (5..30). Позиция порог не
// двигает вовсе — это ушло в vantage.
func TestAuthorGateMapsToCanonicalThreshold(t *testing.T) {
	for band, want := range map[string]int{
		"very_easy": 5, "easy": 10, "normal": 15, "hard": 20, "very_hard": 25, "extreme": 30,
	} {
		got := ThresholdFor(DifficultyFor(core.Intent{}, core.SceneView{GateThreshold: band}))
		if got != want {
			t.Errorf("гейт %q → порог %d, ожидался %d", band, got, want)
		}
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

// Resolve обязан брать порог через функцию (DifficultyFor→ThresholdFor), а не из
// гейта напрямую. Без этого теста связка тихо разъедется: функция останется
// правильной и неиспользуемой. Позиция порог не трогает (она в vantage), но
// авторский гейт — задаёт.
func TestResolveTakesThresholdViaFunction(t *testing.T) {
	sheet := []byte(`{"attrs":{"mind":0},"tags":[]}`)

	// Авторский гейт hard → порог 20, и позиция (Opposed) его не сдвигает.
	gated := New().Resolve(core.Intent{Verb: "question"},
		core.SceneView{GateThreshold: "hard", Opposed: true, Sheet: sheet}, dice.Fixed(10))
	if gated.Log.Threshold != ThresholdHard {
		t.Errorf("порог по авторскому гейту %d, ожидался %d", gated.Log.Threshold, ThresholdHard)
	}

	// Без гейта позиция даёт medium — среда ушла в vantage, порог не двигает.
	for _, v := range []core.SceneView{
		{Opposed: true, Sheet: sheet},
		{Exposed: true, Sheet: sheet},
	} {
		got := New().Resolve(core.Intent{Verb: "question"}, v, dice.Fixed(10))
		if got.Log.Threshold != ThresholdNormal {
			t.Errorf("позиция сдвинула порог до %d, ожидался medium %d", got.Log.Threshold, ThresholdNormal)
		}
	}
}
