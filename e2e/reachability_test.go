package e2e

import (
	"math/rand"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// TestReachabilityNeverCollapses гоняет случайного агента и проверяет, что все
// четыре слота обвинения остаются собираемыми. Рукописное дело разрешимо по
// построению — этот тест ловит момент, когда правка данных это сломала.
func TestReachabilityNeverCollapses(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		g := newGame(t, seed)
		r := rand.New(rand.NewSource(seed))
		playRandomly(g, r, 120)

		if !slotsReachable(g) {
			t.Fatalf("seed %d: дело стало нерешаемым", seed)
		}
	}
}

// playRandomly делает случайные осмысленные ходы: перебирает известные темы и
// присутствующих держателей, иногда переходит между узлами.
func playRandomly(g *core.Game, r *rand.Rand, turns int) {
	verbs := []core.Verb{"examine", "question", "search", "stake_out", "cross_reference"}
	for i := 0; i < turns; i++ {
		here := g.DB.EntitiesAt(g.Node)
		bank := g.K.TopicBank()
		if len(here) == 0 || len(bank) == 0 {
			moveRandomly(g, r)
			continue
		}
		in := core.Intent{
			Verb:  verbs[r.Intn(len(verbs))],
			Actor: g.Actor,
			Args: core.Args{
				Target: here[r.Intn(len(here))].ID,
				Topic:  bank[r.Intn(len(bank))],
			},
		}
		g.Apply(in)
		tryCompares(g)
		if r.Intn(4) == 0 {
			moveRandomly(g, r)
		}
	}
}

func moveRandomly(g *core.Game, r *rand.Rand) {
	reach := g.ReachableNodes()
	if len(reach) == 0 {
		return
	}
	g.Node = reach[r.Intn(len(reach))]
}

func tryCompares(g *core.Game) {
	bank := g.K.TopicBank()
	for i := range bank {
		for j := i + 1; j < len(bank); j++ {
			g.Compare(bank[i], bank[j])
		}
	}
}

// slotsReachable проверяет, что каждый слот обвинения либо уже собираем, либо
// остаётся достижимым: держатель нужного факта жив и стоит в мире.
func slotsReachable(g *core.Game) bool {
	for _, f := range []store.FactID{
		"f_toke_lied", "f_seal_cord", "f_tide_night", "f_shortfall",
	} {
		if g.K.Knows(f) {
			continue
		}
		alive := false
		for _, h := range g.DB.HoldersOf(f) {
			if _, ok := g.DB.Entities[h.HolderID]; ok {
				alive = true
			}
		}
		if !alive && !revealedByCompare(g, f) {
			return false
		}
	}
	return true
}

func revealedByCompare(g *core.Game, f store.FactID) bool {
	for _, c := range g.DB.Contradictions {
		if c.Reveals == f {
			return true
		}
	}
	return false
}

// Каждое место дела обязано становиться известным одним из трёх путей:
// брифинг (start_places), факт (fact_unlocks вида node) или рассказ соседа —
// а рассказ соседа работает только В ОДИН ШАГ от места, куда игрок и так
// доказуемо попадает без рассказа (это ограничение спеки: персонаж
// рассказывает про место, смежное тому, где он стоит, а не про весь свет по
// цепочке). Транзитивное замыкание здесь было бы вакуумным тестом: на
// связном графе оно склеивает всё со всем независимо от того, объявлено ли
// место в start_places, и не поймало бы забытый узел.
func TestEveryPlaceCanBecomeKnown(t *testing.T) {
	for name, path := range caseFiles {
		t.Run(name, func(t *testing.T) {
			cfg, err := cases.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			base := map[store.NodeID]bool{cfg.Start: true}
			for _, n := range cfg.StartPlaces {
				base[n] = true
			}
			for _, us := range cfg.DB.Unlocks {
				for _, u := range us {
					if u.UnlocksKind == "node" {
						base[store.NodeID(u.UnlocksID)] = true
					}
				}
			}
			// Допустимо — база плюс один шаг соседства от неё, без повторного
			// расширения от новых мест.
			known := map[store.NodeID]bool{}
			for n := range base {
				known[n] = true
			}
			for n := range base {
				for _, a := range cfg.DB.Locations[n].Adjacent {
					known[a] = true
				}
			}
			for id := range cfg.DB.Locations {
				if !known[id] {
					t.Errorf("место %s недостижимо: не в start_places, не открыто фактом и не смежно тому, куда игрок попадает без рассказа", id)
				}
			}
		})
	}
}
