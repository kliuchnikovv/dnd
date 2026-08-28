package e2e

import (
	"math/rand"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

var affordanceCases = []string{
	"../cases/harbour/case.json",
	"../cases/forte_merlo/case.json",
}

// Набор подсказок не выдаёт ни правды дела, ни того, чего парти ещё не знает.
//
// Утечка здесь живёт не в формулировке, а в САМОМ НАБОРЕ: список, где одна
// опция ведёт к разгадке, спойлерит безупречными словами (ADR-0003, T2). Тест
// гоняет случайного агента, чтобы состояние знания успело измениться: набор на
// старте безопасен тривиально, а опасен он становится по мере того, как в
// party_knowledge появляются факты.
func TestAffordancesNeverLeak(t *testing.T) {
	for _, path := range affordanceCases {
		g := affordanceGame(t, path)
		r := rand.New(rand.NewSource(7))
		for turn := 0; turn < 200; turn++ {
			checkAffordances(t, path, g)
			playRandomly(g, r, 1)
		}
	}
}

func checkAffordances(t *testing.T, path string, g *core.Game) {
	t.Helper()
	if n := len(g.Affordances("")); n > 4 {
		t.Fatalf("%s: в наборе %d вариантов — это уже меню", path, n)
	}
	for _, a := range g.Affordances("") {
		if topic := a.Intent.Args.Topic; topic != "" && !g.K.Knows(topic) {
			t.Fatalf("%s: в набор попала неизвестная тема %q", path, topic)
		}
		if node := a.Intent.Args.Node; node != "" && !g.KnowsPlace(node) {
			t.Fatalf("%s: в набор попало неизвестное место %q", path, node)
		}
		if item := a.Intent.Args.Item; item != "" && !g.Carries(store.ItemID(item)) {
			t.Fatalf("%s: в набор попал ненесомый предмет %q", path, item)
		}
		// Вариант, который ядро отклонит, — это ложь меню, и она же признак
		// того, что набор поехал за данными дела: отбор по держателям
		// одновременно и разметил бы авторские цели.
		if res := g.Check(a.Intent); res.Refused {
			t.Fatalf("%s: вариант %s → %q отклонён ядром: %s",
				path, a.Intent.Verb, a.Intent.Args.Target, res.Refusal)
		}
	}
}

// Цели осмотра — это детали узла в порядке идентификатора, и ничего кроме.
// Отбор по держателям разметил бы, где лежит авторский контент: половина
// деталей в каждом узле инертна намеренно, и узнать, какая именно, можно
// только потрогав (камуфляжный инвариант).
func TestAffordanceExamineFollowsPropsNotHolders(t *testing.T) {
	for _, path := range affordanceCases {
		g := affordanceGame(t, path)
		for _, node := range g.DB.Locations {
			g.MoveTo(node.ID)
			props := g.DB.Props[node.ID]
			if len(props) == 0 {
				continue
			}
			first := props[0].ID
			for _, p := range props {
				if p.ID < first {
					first = p.ID
				}
			}
			for _, a := range g.Affordances("") {
				if a.Intent.Verb != "examine" {
					continue
				}
				if a.Intent.Args.Target != store.EntityID(first) {
					t.Errorf("%s %s: осмотр предложен для %q, а первая деталь — %q",
						path, node.ID, a.Intent.Args.Target, first)
				}
			}
		}
	}
}

// Каждое имя в наборе игрок и так видит. Это и есть определение
// leak-безопасности набора: он не может назвать ничего, чего нет в read scope —
// присутствующих, деталей места, известных мест, носимого.
//
// Совпадение строк с правдой дела тут не мерится намеренно: токен обвинения
// «emilien» — это идентификатор человека, который стоит в сцене и печатается в
// списке целей с первого хода. Утечка — это назвать то, чего игрок не видит, а
// не совпасть подстрокой с тем, что он видит.
func TestAffordancesNameOnlyWhatThePlayerSees(t *testing.T) {
	for _, path := range affordanceCases {
		g := affordanceGame(t, path)
		r := rand.New(rand.NewSource(11))
		for turn := 0; turn < 100; turn++ {
			visible := readScopeOf(g)
			for _, a := range g.Affordances("") {
				for _, name := range []string{
					string(a.Intent.Args.Target), string(a.Intent.Args.Topic),
					string(a.Intent.Args.Node), a.Intent.Args.Item,
				} {
					if name != "" && !visible[name] {
						t.Fatalf("%s: в наборе назван %q, которого игрок не видит", path, name)
					}
				}
			}
			playRandomly(g, r, 1)
		}
	}
}

// Меню не врёт: ни один вариант, включая реплики, ядро не отклоняет.
//
// Оговорка, без которой тест был бы ложно-зелёным: Check покрывает не все
// отказы. Отказы предъявления живут в Apply — present проверяет предмет и
// адресата уже внутри хода. Исполнить ход здесь нельзя, он сбил бы прогон,
// поэтому его условия проверяются напрямую.
func TestNoAffordanceIsRefused(t *testing.T) {
	for _, path := range affordanceCases {
		g := affordanceGame(t, path)
		r := rand.New(rand.NewSource(3))
		for turn := 0; turn < 100; turn++ {
			for _, npc := range append(npcsAt(g), "") {
				for _, a := range g.Affordances(npc) {
					if res := g.Check(a.Intent); res.Refused {
						t.Fatalf("%s: вариант %s → %q отклонён: %s",
							path, a.Intent.Verb, a.Intent.Args.Target, res.Refusal)
					}
					if a.Intent.Verb != "present" {
						continue
					}
					if !g.Carries(store.ItemID(a.Intent.Args.Item)) {
						t.Fatalf("%s: предложено предъявить ненесомое %q",
							path, a.Intent.Args.Item)
					}
					if a.Intent.Args.Target == "" {
						t.Fatalf("%s: предъявление предложено без адресата", path)
					}
				}
			}
			playRandomly(g, r, 1)
		}
	}
}

// Реплика называет только то, что игрок и так видит либо знает.
func TestRepliesNameOnlyWhatThePlayerKnows(t *testing.T) {
	for _, path := range affordanceCases {
		g := affordanceGame(t, path)
		r := rand.New(rand.NewSource(5))
		for turn := 0; turn < 100; turn++ {
			visible := readScopeOf(g)
			for _, npc := range npcsAt(g) {
				for _, a := range g.Affordances(npc) {
					if !a.Reply {
						continue
					}
					for _, name := range []string{
						string(a.Intent.Args.Target), string(a.Intent.Args.Topic),
						a.Intent.Args.Item,
					} {
						if name != "" && !visible[name] {
							t.Fatalf("%s: реплика назвала %q, которого игрок не видит",
								path, name)
						}
					}
				}
			}
			playRandomly(g, r, 1)
		}
	}
}

func npcsAt(g *core.Game) []store.EntityID {
	var out []store.EntityID
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if e.Kind == store.EntityNPC {
			out = append(out, e.ID)
		}
	}
	return out
}

// readScopeOf — всё, что игрок видит своими глазами: read scope и ничего кроме.
// Граф держателей, факты дела и правда сюда не входят — их тут физически нет.
func readScopeOf(g *core.Game) map[string]bool {
	out := map[string]bool{}
	for _, e := range g.DB.EntitiesAt(g.Node) {
		out[string(e.ID)] = true
	}
	for _, p := range g.DB.Props[g.Node] {
		out[string(p.ID)] = true
	}
	for _, f := range g.K.TopicBank() {
		out[string(f)] = true
	}
	for _, n := range g.KnownPlaces() {
		out[string(n)] = true
	}
	for _, item := range g.Carried() {
		out[string(item.ID)] = true
	}
	return out
}

func affordanceGame(t *testing.T, path string) *core.Game {
	t.Helper()
	cfg, err := cases.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(7).Stream("resolve")
	return core.NewGame(*cfg)
}
