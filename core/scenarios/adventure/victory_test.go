package adventure

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// newGameWithSpec — минимальная игра с зарегистрированным предметом и
// сценарием, настроенным на переданную victory-спецификацию.
func newGameWithSpec(t *testing.T, spec VictorySpec) *core.Game {
	t.Helper()
	db := store.NewDB()
	if spec.Item != "" {
		db.Items[spec.Item] = store.Item{ID: spec.Item}
	}
	sc := &scenario{victory: spec}
	return core.NewGame(core.Config{DB: db, Scenario: sc, Start: "n_start"})
}

func TestVictoryReturnWith(t *testing.T) {
	spec := VictorySpec{Type: "return_with", Item: "i_amulet", Node: "n_start"}
	g := newGameWithSpec(t, spec)
	g.MoveTo("n_start")
	g.Acquire("i_amulet")

	sc := &scenario{victory: spec}
	won, _ := sc.Victory(g)
	if !won {
		t.Fatal("должна быть победа")
	}
}

func TestVictoryWrongNodeOrItem(t *testing.T) {
	spec := VictorySpec{Type: "return_with", Item: "i_amulet", Node: "n_start"}

	t.Run("нет предмета", func(t *testing.T) {
		g := newGameWithSpec(t, spec)
		g.MoveTo("n_start")

		sc := &scenario{victory: spec}
		if won, _ := sc.Victory(g); won {
			t.Fatal("без предмета победы быть не должно")
		}
	})

	t.Run("не тот узел", func(t *testing.T) {
		g := newGameWithSpec(t, spec)
		g.Acquire("i_amulet")
		g.MoveTo("n_elsewhere")

		sc := &scenario{victory: spec}
		if won, _ := sc.Victory(g); won {
			t.Fatal("не в целевом узле — победы быть не должно")
		}
	})
}
