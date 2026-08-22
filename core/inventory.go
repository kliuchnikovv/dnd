package core

import "github.com/kliuchnikovv/dnd/store"

// Инвентарь парти. Домен и только домен: несёт ли детектив предписание —
// вопрос детерминированный, и отвечать на него должен движок, а не модель.
//
// Предмет — не новая механика, а ключ к старым: эффект задаёт автор дела в
// данных, ядро лишь проверяет наличие.

// Carries — несёт ли парти этот предмет.
func (g *Game) Carries(id store.ItemID) bool { return g.DB.HasItem(g.party, id) }

// Acquire кладёт предмет в инвентарь парти. Предмет, которого нет в деле, не
// берётся: иначе инвентарь станет местом, где вещи появляются из воздуха, и
// гейт на такой предмет откроется без авторского на то разрешения.
func (g *Game) Acquire(id store.ItemID) bool {
	if _, ok := g.DB.Items[id]; !ok {
		return false
	}
	g.DB.AddItem(g.party, id)
	return true
}

// Carried — что парти несёт, в стабильном порядке.
func (g *Game) Carried() []store.Item { return g.DB.ItemsOf(g.party) }

// Item — предмет дела по идентификатору.
func (g *Game) Item(id store.ItemID) (store.Item, bool) {
	item, ok := g.DB.Items[id]
	return item, ok
}
