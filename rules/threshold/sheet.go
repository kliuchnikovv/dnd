// Package threshold — система правил «Порог». Заменяемая часть проекта:
// ядро видит её только через интерфейс core.RuleSystem.
package threshold

import (
	"encoding/json"

	"github.com/kliuchnikovv/dnd/core"
)

// Tag — тег персонажа. Применимость проверяется по спискам глаголов и тегов
// обстановки, а не по смыслу: семантическое суждение здесь недопустимо.
type Tag struct {
	Name     string   `json:"name"`
	Verbs    []string `json:"verbs"`
	NodeTags []string `json:"node_tags"`
}

type Sheet struct {
	Archetype string         `json:"archetype"`
	Attrs     map[string]int `json:"attrs"` // body, edge, mind, will
	Tags      []Tag          `json:"tags"`
	Abilities []string       `json:"abilities"`
}

func ParseSheet(raw json.RawMessage) (Sheet, error) {
	var s Sheet
	if len(raw) == 0 {
		return Sheet{Attrs: map[string]int{}}, nil
	}
	err := json.Unmarshal(raw, &s)
	if s.Attrs == nil {
		s.Attrs = map[string]int{}
	}
	return s, err
}

// classAttr связывает класс глагола с атрибутом. Таблица закрыта: новый класс
// без строки здесь читает нулевой атрибут, а не «что-нибудь похожее».
var classAttr = map[core.VerbClass]string{
	core.ClassInvestigate: "mind",
	core.ClassReason:      "mind",
	core.ClassResource:    "mind",
	core.ClassSocial:      "will",
	core.ClassSupport:     "will",
	core.ClassMove:        "edge",
	core.ClassSkill:       "edge",
	core.ClassAttack:      "body",
}

func (s Sheet) Attr(v core.Verb) int {
	def, ok := core.Verbs[v]
	if !ok {
		return 0
	}
	return s.Attrs[classAttr[def.Class]]
}

// TagBonus даёт +2, если хотя бы один тег подходит. Теги не складываются.
func (s Sheet) TagBonus(v core.Verb, view core.SceneView) int {
	for _, tag := range s.Tags {
		for _, tv := range tag.Verbs {
			if core.Verb(tv) == v {
				return TagValue
			}
		}
		for _, nt := range tag.NodeTags {
			if view.HasTag(nt) {
				return TagValue
			}
		}
	}
	return 0
}
