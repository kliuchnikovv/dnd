package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// Сигналы позиции ставит ЯДРО из состояния, и проверяется это здесь. Если бы
// Opposed/Exposed приходили из текста модели, рассказчик двигал бы порог
// броска — ровно то, что запрещает ADR-0001.

// Нейтральная сцена не даёт ни одного сигнала. Флаг, висящий по умолчанию, не
// несёт информации: он бы просто сдвинул всю шкалу сложности.
func TestSceneViewHasNoPositionSignalsByDefault(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	view := g.SceneView(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if view.Opposed {
		t.Error("нейтральная цель отмечена как противодействующая")
	}
	if view.Exposed {
		t.Error("нейтральная цель отмечена как открытая")
	}
}

// Противодействие — свойство ЦЕЛИ этого действия, а не настроения сцены.
func TestSceneViewMarksHostileTargetOpposed(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.D.Adjust("e_toke", -3)

	view := g.SceneView(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if !view.Opposed {
		t.Error("враждебная цель не отмечена как противодействующая")
	}
	// Враждебность — не беспомощность: одно не подразумевает другого.
	if view.Exposed {
		t.Error("враждебная цель отмечена как открытая")
	}
}

// Действие без цели противодействия не встречает: мешать некому.
func TestSceneViewWithoutTargetIsNotOpposed(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.D.Adjust("e_toke", -3)

	if g.SceneView(Intent{Verb: "look"}).Opposed {
		t.Error("действие без цели отмечено как встречающее противодействие")
	}
}

// Враждебный НЕ к этому действию сосед по узлу порога не двигает: численное
// превосходство уже учтено слагаемым, и второй раз считать его нельзя.
func TestSceneViewOpposedLooksAtTargetNotNeighbours(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.D.Adjust("e_toke", -3)
	g.DB.Entities["e_smith"] = store.Entity{ID: "e_smith", Kind: store.EntityNPC, Node: "n_quay"}

	view := g.SceneView(Intent{Verb: "question", Args: Args{Target: "e_smith", Topic: "f_open"}})
	if view.Opposed {
		t.Error("нейтральная цель стала противодействующей из-за враждебного соседа")
	}
}

// Проп воли не имеет: инертная деталь не сопротивляется по определению.
func TestSceneViewPropTargetIsNotOpposed(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Props["n_quay"] = []store.SceneProp{{ID: "p_crates", Node: "n_quay", Name: "Ящики"}}

	if g.SceneView(Intent{Verb: "examine", Args: Args{Target: "p_crates"}}).Opposed {
		t.Error("проп отмечен как противодействующий")
	}
}
