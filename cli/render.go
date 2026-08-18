package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
)

type Render struct{}

func (Render) Scene(g *core.Game) string {
	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", g.DB.Locations[g.Node].Name)
	b.WriteString(g.Flavour("look."+string(g.Node)) + "\n")
	for _, e := range g.DB.EntitiesAt(g.Node) {
		fmt.Fprintf(&b, "  · %s (%s)\n", e.Name, e.ID)
	}
	if reach := g.ReachableNodes(); len(reach) > 0 {
		parts := make([]string, len(reach))
		for i, n := range reach {
			parts[i] = string(n)
		}
		fmt.Fprintf(&b, "  → %s\n", strings.Join(parts, ", "))
	}
	return b.String()
}

// Turn печатает исход хода. Отказ и провал оформлены по-разному намеренно:
// отказ не потратил ход, и игрок должен видеть это без раздумий.
func (r Render) Turn(g *core.Game, t core.TurnResult) string {
	if t.Refused {
		return "нельзя: " + t.Refusal + "\n"
	}
	var b strings.Builder
	if t.FlavourKey != "" {
		b.WriteString(g.Flavour(t.FlavourKey) + "\n")
	}
	if t.Res != nil {
		b.WriteString(r.roll(*t.Res))
	}
	for _, l := range t.Learned {
		fmt.Fprintf(&b, "  + узнали: %s (от %s)\n", g.DB.Facts[l.Fact].Key, l.From)
	}
	if t.FalseLead {
		b.WriteString("  ! след оказался ложным\n")
	}
	if t.HalfEffect {
		b.WriteString("  ~ помогло только наполовину\n")
	}
	for _, c := range t.Fired {
		b.WriteString("  ⏱ " + g.Flavour(c.FlavourKey) + "\n")
	}
	return b.String()
}

func (Render) roll(res core.Resolution) string {
	var terms []string
	for _, t := range res.Log.Terms {
		if t.Value != 0 {
			terms = append(terms, fmt.Sprintf("%s %+d", t.Name, t.Value))
		}
	}
	return fmt.Sprintf("  [d20=%d %s против %d] %s, маржа %+d\n",
		res.Log.Die, strings.Join(terms, " "), res.Log.Threshold, res.Class, res.Margin)
}

// Facts — единственный интерфейс к корроборации. По нему видно, зачем нужен
// третий независимый источник.
func (Render) Facts(g *core.Game) string {
	bank := g.K.TopicBank()
	if len(bank) == 0 {
		return "парти пока ничего не знает\n"
	}
	var b strings.Builder
	for _, f := range bank {
		srcs := g.K.Sources(f)
		names := make([]string, len(srcs))
		for i, s := range srcs {
			names[i] = string(s)
		}
		sort.Strings(names)
		mark := " "
		if g.K.Corroborated(f) {
			mark = "*"
		}
		fmt.Fprintf(&b, "%s %-24s %.3f  ← %s\n",
			mark, f, g.K.Confidence(f), strings.Join(names, ", "))
	}
	b.WriteString("(* — подтверждён тремя независимыми источниками)\n")
	return b.String()
}

func (Render) State(g *core.Game) string {
	ch := g.DB.Characters[g.Actor]
	return fmt.Sprintf("узел: %s\nранения: %d/3\ngrit: %d\nпопыток обвинения: %d\n",
		g.Node, ch.Harm, ch.Grit, g.Attempts)
}

func (Render) Clocks(g *core.Game) string {
	var b strings.Builder
	for _, c := range g.C.Snapshot() {
		fmt.Fprintf(&b, "%-20s [%s%s] %d/%d\n", c.Name,
			strings.Repeat("#", c.Filled), strings.Repeat(".", c.Segments-c.Filled),
			c.Filled, c.Segments)
	}
	return b.String()
}

func (Render) Help() string {
	return `question <источник> <тема>      расспросить
examine <предмет>               осмотреть
search <локация>                обыскать
compare <факт> <факт>           сопоставить
cross_reference <факт> <запись> сверить
stake_out <локация>             наблюдать
theorize <текст>                зафиксировать гипотезу
move_zone <узел>                перейти
accuse                          открыть форму обвинения
facts                           известные факты с источниками и confidence
state                           узел, часы, ранения, grit, попытки
clocks                          часы давления
help                            этот список
quit                            выйти
`
}
