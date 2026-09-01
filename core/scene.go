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

	// Opposed и Exposed — позиция цели. Их ставит ЯДРО из состояния сцены, а
	// не текст модели: порог броска — не то, что рассказчик вправе двигать
	// (ADR-0001). Оба нужны там, где авторского гейта нет и сложность
	// приходится судить: без них удар по открытой в упор цели и та же попытка
	// против сопротивляющегося противника получали бы одну базовую сложность.
	//
	// Opposed — у цели есть воля и намерение мешать ИМЕННО этому действию.
	Opposed bool
	// Exposed — цель открыта или беспомощна: оглушена, связана, не защищается.
	Exposed bool
	// Sheet — лист персонажа. Для ядра это байты; разбирает их система правил.
	Sheet json.RawMessage

	// TargetAC — класс доспеха цели для attack-резолва (dnd5e). Ноль, если
	// действие не боевое или AC ещё не проставлен авторством кейса.
	TargetAC int
	// DC — сложность проверки/спасброска, заявленная данными дела. Ноль
	// означает «не проставлена явно» — резолвер тогда идёт в CheckDC.
	DC int
	// SaveAbility — какая характеристика спасается ("str"|"dex"|...) для
	// save-резолва. Пусто, если сцена не про спасбросок.
	SaveAbility string
}

// CheckDC — сложность проверки по verb/target из секции "checks:" данных
// дела. Пока заглушка: плумбинг case.json — задача авторства, не резолвера
// (Task 12). Возвращает 0, пока таблица не подключена; резолвер тогда
// использует уже проставленный SceneView.DC.
func (v SceneView) CheckDC(verb, target string) int {
	return 0
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
