package intent

import (
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

type Named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SceneHint — всё, что парсер знает о мире. Строится ИСКЛЮЧИТЕЛЬНО из того,
// что игрок и так видит: сущности текущего узла, темы из банка знаний парти,
// открытые смежные узлы. Правды дела здесь нет и быть не может — как и в
// SceneView, который получает система правил.
type SceneHint struct {
	Node      string  `json:"node"`
	NodeName  string  `json:"node_name"`
	Entities  []Named `json:"entities"`
	Topics    []Named `json:"topics"`
	Reachable []Named `json:"reachable"`
}

// BuildHint собирает подсказку из игры. Единственный источник тем —
// TopicBank, то есть party_knowledge: защита от угадывания сохраняется на
// входе, а не только при резолве.
func BuildHint(g *core.Game) SceneHint {
	h := SceneHint{Node: string(g.Node), NodeName: g.DB.Locations[g.Node].Name}
	for _, e := range g.DB.EntitiesAt(g.Node) {
		h.Entities = append(h.Entities, Named{string(e.ID), e.Name})
	}
	for _, f := range g.K.TopicBank() {
		h.Topics = append(h.Topics, Named{string(f), g.DB.Facts[f].Key})
	}
	for _, n := range g.ReachableNodes() {
		h.Reachable = append(h.Reachable, Named{string(n), g.DB.Locations[n].Name})
	}
	sort.Slice(h.Entities, func(i, j int) bool { return h.Entities[i].ID < h.Entities[j].ID })
	return h
}

func (h SceneHint) hasEntity(id string) bool { return contains(h.Entities, id) }
func (h SceneHint) hasTopic(id string) bool  { return contains(h.Topics, id) }
func (h SceneHint) hasNode(id string) bool   { return contains(h.Reachable, id) }

func contains(list []Named, id string) bool {
	for _, n := range list {
		if n.ID == id {
			return true
		}
	}
	return false
}

// Render — подсказка в виде, который уходит в промпт. Компактно и стабильно:
// нестабильный порядок обнулял бы кэш префикса.
func (h SceneHint) Render() string {
	var b strings.Builder
	b.WriteString("Узел: " + h.Node + " (" + h.NodeName + ")\n")
	b.WriteString("Присутствуют:\n")
	for _, e := range h.Entities {
		b.WriteString("  " + e.ID + " — " + e.Name + "\n")
	}
	if len(h.Topics) == 0 {
		b.WriteString("Известные темы: ничего\n")
	} else {
		b.WriteString("Известные темы:\n")
		for _, t := range h.Topics {
			b.WriteString("  " + t.ID + " — " + t.Name + "\n")
		}
	}
	b.WriteString("Проходы:\n")
	for _, n := range h.Reachable {
		b.WriteString("  " + n.ID + " — " + n.Name + "\n")
	}
	return b.String()
}

var _ = store.FactID("")
