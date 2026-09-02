package adventure

import (
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// VictorySpec — условие победы приключения, задаётся полем victory: в
// case.json. Пока единственный тип — "return_with": вернуться в узел Node,
// неся при себе Item. Пустой Type — кейс без победы (обратная совместимость
// с делами, у которых этого поля нет).
type VictorySpec struct {
	Type string       `json:"type"`
	Item store.ItemID `json:"item"`
	Node store.NodeID `json:"node"`
}

// check — единственная точка правды об условии победы. Без типа — победы
// не бывает: неполная спецификация не должна молча объявлять выигрыш.
func (v VictorySpec) check(g *core.Game) (bool, string) {
	switch v.Type {
	case "return_with":
		if g.Node == v.Node && g.Carries(v.Item) {
			return true, "цель выполнена: " + string(v.Item) + " возвращён"
		}
	}
	return false, ""
}
