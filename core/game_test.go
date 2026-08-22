package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// Инвентарь парти в ядре: детектив либо несёт бумагу, либо нет, и это
// детерминированный вопрос, а не суждение модели.
func TestCarriesAndAcquire(t *testing.T) {
	db := store.NewDB()
	db.Items["i_writ"] = store.Item{ID: "i_writ", Kind: "credential", Name: "Предписание магистрата"}
	g := NewGame(Config{DB: db})

	if g.Carries("i_writ") {
		t.Error("парти несёт то, чего ей не давали")
	}
	g.Acquire("i_writ")
	if !g.Carries("i_writ") {
		t.Error("выданный предмет не оказался в инвентаре")
	}
}

// Предмета, которого нет в деле, взять нельзя: иначе инвентарь становится
// местом, где предметы появляются из воздуха.
func TestAcquireIgnoresUnknownItem(t *testing.T) {
	g := NewGame(Config{DB: store.NewDB()})
	g.Acquire("i_нет_такого")
	if g.Carries("i_нет_такого") {
		t.Error("в инвентарь попал предмет, которого нет в деле")
	}
}

// Инвентарь читается в стабильном порядке: список «что несёшь» обязан быть
// воспроизводимым, а итерация по map в Go случайна.
func TestCarriedReadsInStableOrder(t *testing.T) {
	db := store.NewDB()
	db.Items["i_writ"] = store.Item{ID: "i_writ", Name: "Предписание"}
	db.Items["i_lamp"] = store.Item{ID: "i_lamp", Name: "Фонарь"}
	g := NewGame(Config{DB: db})
	g.Acquire("i_writ")
	g.Acquire("i_lamp")

	got := g.Carried()
	if len(got) != 2 || got[0].ID != "i_lamp" || got[1].ID != "i_writ" {
		t.Errorf("порядок инвентаря нестабилен: %+v", got)
	}
}
