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
	// Tools — пропы текущего узла, которыми можно воспользоваться. Без них
	// use_item — тупик: обязательный аргумент есть, а взять его негде, и игра
	// спрашивает «чем именно?», не имея ответа.
	Tools []Named `json:"tools"`
	// Carried — что парти несёт. Отдельно от Tools, и это не педантизм:
	// use_item резолвится только по пропам узла с меткой tool, поэтому
	// носимая бумага, влитая в Tools, вернула бы этот глагол в грамматику — и
	// модель уехала бы в бросок по пропу, которого в узле нет.
	Carried []Named `json:"carried"`
	// Talk — идущий разговор. Часть сцены: фраза игрока чаще всего продолжает
	// разговор, а не начинает ход с нуля.
	Talk Talk `json:"talk"`
}

// Talk — состояние разговора, в котором произнесена фраза.
//
// Без него разбор не понимает ответа на свой же вопрос: «достаю из кармана»
// после просьбы предъявить предписание читается как загадка, а «рукой» после
// вопроса «чем именно?» — как новое действие. Игрок при этом уверен, что
// продолжает один разговор, и он прав.
type Talk struct {
	// With — с кем идёт разговор.
	With store.EntityID `json:"with"`
	// Recent — последние круги, от старого к новому.
	Recent []store.Exchange `json:"recent"`
	// Pending — вопрос, который игре задала сама игра и на который сейчас
	// отвечает игрок.
	Pending string `json:"pending"`
}

// empty сообщает, что разговора нет и печатать в промпт нечего: пустая секция
// это шум, за который платят токенами каждый ход.
func (t Talk) empty() bool {
	return t.With == "" && len(t.Recent) == 0 && strings.TrimSpace(t.Pending) == ""
}

// render — разговор в виде, который уходит в промпт.
func (t Talk) render() string {
	if t.empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nРазговор идёт")
	if t.With != "" {
		b.WriteString(" с " + string(t.With))
	}
	b.WriteString(":\n")
	for _, e := range t.Recent {
		if e.Player != "" {
			b.WriteString("  игрок: " + e.Player + "\n")
		}
		if e.Reply != "" {
			b.WriteString("  он: " + e.Reply + "\n")
		}
	}
	if q := strings.TrimSpace(t.Pending); q != "" {
		b.WriteString("Ты спросил игрока: " + q + "\n" +
			"Фраза ниже отвечает на этот вопрос, а не начинает новое действие. " +
			"Если ответ всё равно не ложится на глагол — unsupported, а не clarify " +
			"тем же вопросом: спрашивать второй раз то же самое значит зациклить игру.\n")
	}
	return b.String()
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
	// Инструмент — вещь вида tool: проп этого узла или носимый предмет. Ровно
	// то, что примет движок: разойдись эти списки, игрок «применял» бы бочки.
	for _, p := range g.DB.Props[g.Node] {
		if p.Kind == store.ToolKind {
			h.Tools = append(h.Tools, Named{string(p.ID), p.Name})
		}
	}
	for _, item := range g.Carried() {
		if item.Kind == store.ToolKind {
			h.Tools = append(h.Tools, Named{string(item.ID), item.Name})
		}
	}
	sort.Slice(h.Tools, func(i, j int) bool { return h.Tools[i].ID < h.Tools[j].ID })
	// Инвентарь читается из ядра в стабильном порядке — подсказка обязана
	// собираться одинаково, иначе кэш префикса у провайдера обнуляется каждый
	// ход.
	for _, item := range g.Carried() {
		h.Carried = append(h.Carried, Named{string(item.ID), item.Name})
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
	if len(h.Tools) > 0 {
		b.WriteString("Можно применить:\n")
		for _, tool := range h.Tools {
			b.WriteString("  " + tool.ID + " — " + tool.Name + "\n")
		}
	}
	if len(h.Carried) > 0 {
		b.WriteString("При себе (это можно предъявить):\n")
		for _, item := range h.Carried {
			b.WriteString("  " + item.ID + " — " + item.Name + "\n")
		}
	}
	b.WriteString(h.Talk.render())
	return b.String()
}
