package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
)

type Render struct{}

func (r Render) Scene(g *core.Game) string {
	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", g.DB.Locations[g.Node].Name)
	fmt.Fprintf(&b, "%s\n", g.Flavour("look."+string(g.Node)))
	b.WriteString(targetList(g))
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
		fmt.Fprintf(&b, "%s\n", g.Flavour(t.FlavourKey))
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
		fmt.Fprintf(&b, "  ⏱ %s\n", g.Flavour(c.FlavourKey))
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
	var b strings.Builder
	fmt.Fprintf(&b, "узел: %s\nранения: %d/%d\ngrit: %d\nпопыток обвинения: %d\n",
		g.Node, ch.Harm, core.HarmMax, ch.Grit, g.Attempts)
	if g.Incapacitated() {
		b.WriteString("выведен из строя: доступны только осмотр и отдых\n")
	}
	if len(g.Theories) > 0 {
		fmt.Fprintf(&b, "гипотезы:\n")
		for i, th := range g.Theories {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, th)
		}
	}
	return b.String()
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
	return `survey                          что здесь можно трогать
push <действие>                 потратить очко grit: +2 к этому броску
question <источник> <тема>      расспросить
examine <предмет>               осмотреть
search <локация>                обыскать
compare <факт> <факт>           сопоставить
cross_reference <факт> <запись> сверить
stake_out <локация>             наблюдать
theorize <текст>                зафиксировать гипотезу
«реплика» или — реплика        сказать вслух: прямая речь персонажа
move_zone <узел>                перейти
rest short|long                 короткий или длинный отдых
accuse                          открыть форму обвинения
facts                           известные факты с источниками и confidence
state                           узел, часы, ранения, grit, попытки
clocks                          часы давления
help                            этот список
quit                            выйти
`
}

// Survey печатает интерактивные цели узла: холдеров и пропы в одном списке,
// без разметки, в стабильном порядке по имени.
//
// Разметка здесь была бы карту решения: игрок, видящий, какие четыре цели
// «настоящие», не расследует, а перебирает. Половина списка инертна, и узнать,
// какая именно, можно только потрогав.
func (Render) Survey(g *core.Game) string {
	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", g.DB.Locations[g.Node].Name)
	b.WriteString(targetList(g))
	return b.String()
}

// targetList — холдеры и пропы одним перечнем, отсортированные по имени.
// Единственное место, где строится список целей: сцена и survey обязаны
// печатать одно и то же, иначе одна из двух команд начнёт выдавать граф.
func targetList(g *core.Game) string {
	type target struct{ name, id string }
	var all []target
	for _, e := range g.DB.EntitiesAt(g.Node) {
		all = append(all, target{e.Name, string(e.ID)})
	}
	for _, p := range g.DB.Props[g.Node] {
		all = append(all, target{p.Name, string(p.ID)})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].name != all[j].name {
			return all[i].name < all[j].name
		}
		return all[i].id < all[j].id
	})

	var b strings.Builder
	for _, t := range all {
		fmt.Fprintf(&b, "  · %s (%s)\n", t.name, t.id)
	}
	return b.String()
}
