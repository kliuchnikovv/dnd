package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// ProseKind — что описывает проза: обстановку или исход хода.
type ProseKind string

const (
	ProsePlace   ProseKind = "place"
	ProseOutcome ProseKind = "outcome"
)

// Prose — заказ на прозу Мастера.
type Prose struct {
	Kind  ProseKind
	Frame string
	Scene []string
	// Outcome — что произошло, словами без механики.
	Outcome []string
	// Speaking — тот, кто сейчас ответит своей репликой. Мастеру нельзя ни
	// говорить за него, ни описывать, как он себя повёл: реплика идёт
	// следующей строкой, и проза успевала ей противоречить — сперва «он
	// отвечает охотнее, чем ждали», а потом сухое «Что вам надобно?».
	Speaking string
}

type Render struct {
	// Narrate переписывает авторский текст прозой Мастера. Пустой — печатается
	// авторский текст: игра без моделей обязана работать как раньше.
	//
	// Структура остаётся кодовой: заголовок, цели, выходы, бросок и «узнали»
	// печатает презентация, а не модель.
	Narrate func(Prose) string
}

// prose — авторский текст либо его оживлённая версия.
func (r Render) prose(p Prose) string {
	if r.Narrate == nil || strings.TrimSpace(p.Frame) == "" {
		return p.Frame
	}
	return r.Narrate(p)
}

// sceneOf — что видно вокруг. Мастеру это нужно, чтобы не противоречить
// обстановке.
func sceneOf(g *core.Game) []string {
	loc := g.DB.Locations[g.Node]
	out := []string{"Место: " + loc.Name}
	if len(loc.Tags) > 0 {
		out = append(out, "Обстановка: "+strings.Join(loc.Tags, ", "))
	}
	return out
}

func (r Render) Scene(g *core.Game) string {
	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", g.DB.Locations[g.Node].Name)
	fmt.Fprintf(&b, "%s\n", r.prose(Prose{Kind: ProsePlace,
		Frame: g.Flavour("look." + string(g.Node)), Scene: sceneOf(g)}))
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
func (r Render) Turn(g *core.Game, in core.Intent, t core.TurnResult) string {
	if t.Refused {
		return "нельзя: " + t.Refusal + "\n"
	}
	var b strings.Builder
	if t.FlavourKey != "" {
		fmt.Fprintf(&b, "%s\n", r.prose(Prose{Kind: ProseOutcome,
			Frame: g.Flavour(t.FlavourKey), Scene: sceneOf(g),
			Outcome: outcomeOf(g, t), Speaking: speakerName(g, in, t)}))
	}
	// Бросок печатается, только если он был: у безопасного действия кость не
	// трогается, и Log.Die остаётся нулём.
	if t.Res != nil && t.Res.Log.Die != 0 {
		b.WriteString(r.roll(*t.Res))
	}
	for _, l := range t.Learned {
		fmt.Fprintf(&b, "  + узнали: %s (от %s)\n", g.DB.Facts[l.Fact].Key, l.From)
	}
	if names := costNames(t.Costs); names != "" {
		fmt.Fprintf(&b, "  цена: %s\n", names)
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

// speakerName — имя того, к кому обращён ход: он же сейчас и ответит. Пусто,
// если ход обращён не к человеку — или если отвечать никто не будет.
//
// Второе важнее первого. Ход, выдавший факт, озвучку глушит намеренно: там уже
// есть авторская реплика. Запретить Мастеру говорить за молчащего значит
// потерять реакцию вовсе — живой прогон так и потерял ответ стражника на
// предъявленное предписание, оставив прозу про дождь.
func speakerName(g *core.Game, in core.Intent, t core.TurnResult) string {
	if len(t.Learned) > 0 {
		return ""
	}
	e, ok := g.DB.Entities[in.Args.Target]
	if !ok || e.Kind != store.EntityNPC {
		return ""
	}
	return e.Name
}

// outcomeOf — что только что произошло, словами без механики. Мастер
// описывает исход, поэтому исход обязан до него доехать; ключ факта тут
// законен — парти его уже знает, а правды дела в нём нет.
func outcomeOf(g *core.Game, t core.TurnResult) []string {
	var out []string
	if t.Res != nil {
		out = append(out, "исход: "+t.Res.Class.String())
	}
	for _, l := range t.Learned {
		out = append(out, "узнали: "+g.DB.Facts[l.Fact].Key)
	}
	if names := costNames(t.Costs); names != "" {
		out = append(out, "цена: "+names)
	}
	if t.FalseLead {
		out = append(out, "след оказался ложным")
	}
	if t.HalfEffect {
		out = append(out, "помогло только наполовину")
	}
	return out
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

// Items — что парти несёт. Игрок обязан это видеть: предъявлять придётся по
// имени, а угадывать содержимое своих карманов — не игра.
func (Render) Items(g *core.Game) string {
	carried := g.Carried()
	if len(carried) == 0 {
		return "при себе ничего нет\n"
	}
	var b strings.Builder
	for _, item := range carried {
		fmt.Fprintf(&b, "  %-10s %s\n", item.ID, item.Name)
		if item.Text != "" {
			fmt.Fprintf(&b, "             %s\n", item.Text)
		}
	}
	return b.String()
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
	// Легенда обязана сказать не только что значит звёздочка, но и чего она НЕ
	// значит. Плейтест прочитал её как условие обвинения и остановился, имея
	// на руках всё нужное: слот обвинения открывает знание факта, а
	// подтверждение делает дело крепким.
	b.WriteString("(* — подтверждён тремя независимыми источниками; " +
		"обвинять можно и без звёздочки — она про крепость дела, не про право)\n")
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
examine <цель>                  осмотреть
search <цель>                   обыскать
compare <факт> <факт>           сопоставить
cross_reference <запись> <факт> сверить запись с известным фактом
stake_out <цель>                наблюдать
theorize <текст>                зафиксировать гипотезу
«реплика» или — реплика        сказать вслух: прямая речь персонажа
move_zone <узел>                перейти
rest short|long                 короткий или длинный отдых
present <вещь> [кому]           предъявить носимое: бумагу, улику
items                           что несёшь
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

// costNames переводит таксономию ядра на язык игрока. Ложный след и половина
// эффекта сюда не входят: у них своя, более заметная строка, и дублировать её
// в перечне цен незачем. Цена, видимая только
// когда часы заполнятся, — это не цена, а сюрприз через двадцать минут.
var costWords = map[core.CostKind]string{
	core.CostTickClock:       "часы сдвинулись",
	core.CostDebt:            "за вами теперь должок",
	core.CostDispositionDown: "расположение испорчено",
	core.CostPositionWorse:   "вас заметили",
	core.CostHarmSelf:        "ранение",
	core.CostResourceSpent:   "ресурс потрачен впустую",
}

func costNames(cs []core.CostKind) string {
	seen := map[core.CostKind]bool{}
	var out []string
	for _, c := range cs {
		if seen[c] {
			continue
		}
		seen[c] = true
		if w, ok := costWords[c]; ok {
			out = append(out, w)
		}
	}
	return strings.Join(out, ", ")
}
