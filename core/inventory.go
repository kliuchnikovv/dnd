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

// readyTool берёт инструмент в руки: проп текущего узла или носимый предмет,
// если он вида tool. Возвращает ключ флейвора и признак, что инструмент взят.
//
// Две таблицы, одно слово: инструментальность и у пропа, и у предмета живёт в
// Kind. Пока их было две — тег у пропа и вид у предмета, — они успели
// разойтись в данных.
func (g *Game) readyTool(id string) (string, bool) {
	if p, ok := g.DB.PropAt(g.Node, store.PropID(id)); ok && p.Kind == store.ToolKind {
		g.tool, g.toolNode = p.ID, g.Node
		return "prop." + string(p.ID), true
	}
	// Носимый инструмент берётся в руки там, где стоишь: готовность привязана
	// к узлу, потому что платой за неё был ход, сделанный ЗДЕСЬ. Фонарь из
	// кармана не светит в двух местах сразу.
	if item, ok := g.Item(store.ItemID(id)); ok && item.Kind == store.ToolKind && g.Carries(item.ID) {
		g.tool, g.toolNode = store.PropID(item.ID), g.Node
		return "item." + string(item.ID), true
	}
	return "", false
}

// learn — узнать факт и получить то, что он приносит. Обёртка нужна ровно
// затем, чтобы выдача не зависела от пути: факт, услышанный от человека, и
// факт, открытый сопоставлением, приносят предмет одинаково. Три места вызова
// K.Learn забыть проще, чем одно.
func (g *Game) learn(fact store.FactID, from store.EntityID) bool {
	if !g.K.Learn(fact, from) {
		return false
	}
	if item := g.DB.Facts[fact].GrantsItem; item != "" {
		// Предмета может не быть в деле — это опечатка автора, и валидатор
		// ловит её на загрузке. Здесь важно не превратить её в предмет из
		// воздуха, открывающий гейт.
		g.Acquire(item)
	}
	return true
}

// present — след предъявления. Отдельно от выдачи факта: факт отдаёт общая
// ветка без броска, а здесь только то, что делает само предъявление, —
// отметка «показано этому человеку» и авторский сдвиг расположения.
//
// Возвращает (отказ, true), если предъявлять нечего или некому. Отказ, а не
// провал: ход не потрачен, потому что игрок не действовал в мире, а ошибся в
// команде.
func (g *Game) present(in Intent) (TurnResult, bool) {
	item, ok := g.Item(store.ItemID(in.Args.Item))
	if !ok {
		return refuse("такого предмета в деле нет"), true
	}
	if !g.Carries(item.ID) {
		return refuse("этого у тебя при себе нет"), true
	}
	if in.Args.Target == "" {
		// Форма «предъявить узлу» дизайном оставлена на будущее. Молча съесть
		// ход было бы хуже: игрок решил бы, что предъявление не работает вовсе.
		return refuse("предъявлять некому: назови, кому показываешь"), true
	}
	// Сдвиг применяется один раз на предмет: иначе бумагу показывают десять
	// раз и получают дружбу из ничего.
	if g.D.MarkPresented(in.Args.Target, item.ID) && item.DispositionDelta != 0 {
		g.D.Adjust(in.Args.Target, item.DispositionDelta)
	}
	return TurnResult{}, false
}
