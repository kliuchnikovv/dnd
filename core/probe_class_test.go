package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// Класс, предложенный парсером по свободному тексту, — подсказка о ФОРМЕ и
// ничего больше. Проверяется здесь ровно граница из ADR-0001: парсер вправе
// сказать, на что действие похоже, но не вправе решать, что миру можно и по
// какому порогу это бросается.

// Недоверенный вход проверяется по закрытому реестру ядра, а не принимается на
// слово. Класс, которого в реестре нет, не «неизвестный класс», а отсутствие
// подсказки: выдуманное значение не должно уметь ничего.
func TestLookupClassRejectsWhatIsNotInTheRegistry(t *testing.T) {
	if _, ok := LookupClass("attack"); !ok {
		t.Error("класс из реестра не опознан")
	}
	// «none» здесь наравне с выдуманным: класс глаголов, которые ничего не
	// делают, подсказкой о форме быть не может.
	for _, bad := range []string{"", "none", "ATTACK", "рукопашная", "none ", "admin"} {
		if c, ok := LookupClass(bad); ok {
			t.Errorf("выдуманный класс %q принят как %q", bad, c)
		}
	}
}

// probeClassGame — тело с ДВУМЯ авторскими гейтами на разные глаголы. Без этого
// проверить, что подсказка выбирает форму, физически нечем: при одном гейте
// выбирать не из чего.
func probeClassGame() *Game {
	g := probeGame()
	g.DB.Facts["f_wound"] = store.Fact{ID: "f_wound", Key: "рана от удара"}
	g.DB.Holders["f_wound"] = []store.FactHolder{{
		FactID: "f_wound", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"grapple"}, Threshold: "hard"},
	}}
	return g
}

// Без подсказки маршрут прежний: перебор идёт по глаголам наблюдения.
func TestProbeWithoutClassHintKeepsTheOldRoute(t *testing.T) {
	g := probeClassGame()
	in, ok := g.MatchProbeAs(Probe{Text: "щупаю шею у тела"})
	if !ok {
		t.Fatal("проба не легла на авторскую цель")
	}
	if in.Verb != "examine" {
		t.Errorf("глагол пробы без подсказки %q, ожидался examine", in.Verb)
	}
}

// Подсказка выбирает форму среди того, что НАПИСАЛ АВТОР. Она не создаёт
// действий: гейта нет — не легло.
func TestProbeClassHintChoosesTheFormAmongAuthoredGates(t *testing.T) {
	g := probeClassGame()
	in, ok := g.MatchProbeAs(Probe{Text: "наваливаюсь на тело", Class: ClassAttack})
	if !ok {
		t.Fatal("проба с подсказкой не легла на авторскую цель")
	}
	if in.Verb != "grapple" {
		t.Errorf("подсказка attack дала глагол %q, ожидался grapple", in.Verb)
	}
}

// Подсказка класса, за которым автор ничего не положил, не открывает ничего:
// маршрут откатывается к наблюдению, а не выдумывает ход.
func TestProbeClassHintWithoutAuthoredGateFallsBack(t *testing.T) {
	g := probeClassGame()
	in, ok := g.MatchProbeAs(Probe{Text: "щупаю шею у тела", Class: ClassSupport})
	if !ok {
		t.Fatal("проба с бесполезной подсказкой перестала ложиться")
	}
	if in.Verb != "examine" {
		t.Errorf("подсказка support дала глагол %q, ожидался откат на examine", in.Verb)
	}
}

// Выдуманный класс не сдвигает ничего: он даже не доходит до маршрута.
func TestProbeIgnoresAnInventedClass(t *testing.T) {
	g := probeClassGame()
	in, ok := g.MatchProbeAs(Probe{Text: "щупаю шею у тела", Class: VerbClass("admin")})
	if !ok {
		t.Fatal("проба с выдуманным классом перестала ложиться")
	}
	if in.Verb != "examine" {
		t.Errorf("выдуманный класс сдвинул форму на %q", in.Verb)
	}
}

// Подсказка не открывает нелегального хода: Check стоит на том же месте.
func TestProbeClassHintDoesNotBypassLegality(t *testing.T) {
	g := probeClassGame()
	g.DB.Entities["e_body"] = store.Entity{ID: "e_body", Name: "Тело Халдена",
		Kind: store.EntityThing, Node: "n_forge"} // тела здесь больше нет

	if in, ok := g.MatchProbeAs(Probe{Text: "наваливаюсь на тело", Class: ClassAttack}); ok {
		t.Errorf("проба легла на отсутствующую цель ходом %q", in.Verb)
	}
}
