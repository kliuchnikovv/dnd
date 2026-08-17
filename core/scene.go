package core

import (
	"encoding/json"

	"github.com/kliuchnikovv/dnd/store"
)

// SceneView — всё, что система правил видит о мире. Здесь намеренно нет графа
// фактов и нет cases.truth: правила физически не могут их прочитать.
type SceneView struct {
	Node       store.NodeID
	NodeTags   []string // dark, crowd, rain, indoors, ...
	Allies     int
	Foes       int
	Undetected bool
	Cover      bool
	ActorTier  int
	TargetTier int
	Tools      []string
	Harm       int // заполненные ячейки ранений
	Grit       int
	// GateThreshold — сложность, заявленная данными дела: "easy"|"normal"|"hard".
	// Пусто, если действие не привязано к держателю факта.
	GateThreshold string
	// Sheet — лист персонажа. Для ядра это байты; разбирает их система правил.
	Sheet json.RawMessage
}

// HasTag сообщает, присутствует ли тег обстановки. Применимость тегов
// персонажа проверяется по спискам, а не по смыслу.
func (v SceneView) HasTag(tag string) bool {
	for _, t := range v.NodeTags {
		if t == tag {
			return true
		}
	}
	return false
}

func (v SceneView) HasTool(tool string) bool {
	for _, t := range v.Tools {
		if t == tool {
			return true
		}
	}
	return false
}
