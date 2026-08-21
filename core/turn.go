package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

type Learned struct {
	Fact store.FactID
	From store.EntityID
}

// TurnResult — исход хода. Отказ и провал различены намеренно: отказ не
// тратит ход и не тикает часы, и игрок обязан видеть разницу мгновенно.
type TurnResult struct {
	Refused    bool
	Refusal    string
	FlavourKey string
	Learned    []Learned
	Res        *Resolution
	Costs      []CostKind
	Fired      []store.Consequence
	FalseLead  bool
	HalfEffect bool
}

// Apply — пятишаговый ход: валидация, ветка без броска, сборка SceneView,
// Resolve, применение. Ядро никогда не видит кости: они уходят внутрь Resolve.
func (g *Game) Apply(in Intent) TurnResult {
	def, ok := Verbs[in.Verb]
	if !ok {
		return refuse("неизвестное действие")
	}

	// theorize — заметка игрока, а не действие в мире: она не проходит
	// валидацию мира (нет ни цели, ни узла, ни темы), не бросает кубик и не
	// тикает часы.
	if in.Verb == "theorize" {
		if in.Args.Text == "" {
			return refuse("гипотеза не может быть пустой")
		}
		g.Theories = append(g.Theories, in.Args.Text)
		return TurnResult{FlavourKey: "theorize.recorded"}
	}

	// Выведенный из строя не действует. Свободные пробы остаются: иначе это
	// не состояние, а тупик.
	if def.Hard && g.Incapacitated() {
		return refuse("персонаж выведен из строя — сначала отдых")
	}

	// Шаг 1: валидация.
	if r, bad := g.validate(in, def); bad {
		return r
	}

	// Взять инструмент — отдельный ход без броска. Проп с тегом tool ничего не
	// даёт, пока лежит: плата за отмену штрафа среды — потраченное действие.
	if in.Verb == "use_item" {
		if p, ok := g.DB.PropAt(g.Node, store.PropID(in.Args.Item)); ok && hasTag(p, "tool") {
			g.tool, g.toolNode = p.ID, g.Node
			return TurnResult{FlavourKey: "prop." + string(p.ID)}
		}
	}

	holder, found := g.holderFor(in)

	// Шаг 2: ветка без броска.
	if !def.Rolls || (found && holder.Mandatory) {
		res := TurnResult{FlavourKey: g.flavourKeyFor(in, holder, found)}
		if found {
			if g.K.Learn(holder.FactID, holder.HolderID) {
				res.Learned = append(res.Learned, Learned{holder.FactID, holder.HolderID})
				g.applyUnlocksFor(holder.FactID)
			}
		}
		return res
	}

	// Шаг 3: сборка SceneView.
	view := g.SceneView(in)

	// Шаг 4: бросок в системе правил.
	resolution := g.Rules.Resolve(in, view, g.Dice)

	// Шаг 5: применение.
	out := TurnResult{Res: &resolution, Costs: resolution.Costs, FlavourKey: g.flavourKeyFor(in, holder, found)}
	g.applyMutations(resolution.Mutations)
	out.Fired = g.executeCosts(resolution.Costs, in)
	g.applyConsequences(out.Fired)

	for _, c := range resolution.Costs {
		switch c {
		case CostFalseLead:
			out.FalseLead = true
		case CostHalfEffect:
			out.HalfEffect = true
		}
	}

	if found && resolution.Class >= OutcomeSuccess {
		if g.K.Learn(holder.FactID, holder.HolderID) {
			out.Learned = append(out.Learned, Learned{holder.FactID, holder.HolderID})
			g.applyUnlocksFor(holder.FactID)
		}
	}
	return out
}

func (g *Game) validate(in Intent, def VerbDef) (TurnResult, bool) {
	if in.Args.Target != "" && !g.isProp(in.Args.Target) {
		e, ok := g.DB.Entities[in.Args.Target]
		if !ok {
			return refuse("такой сущности в деле нет"), true
		}
		if e.Node != g.Node {
			return refuse("этого нет в текущей локации"), true
		}
	}
	if in.Args.Node != "" {
		if !g.DB.Adjacent(g.Node, in.Args.Node) {
			return refuse("туда отсюда не пройти"), true
		}
		if g.nodeLocked(in.Args.Node) && !g.Unlocked("node", string(in.Args.Node)) {
			return refuse("туда пока незачем идти"), true
		}
	}
	for _, f := range in.Args.Facts {
		if !g.K.Knows(f) {
			return refuse("этот факт парти неизвестен"), true
		}
	}
	if in.Args.Topic != "" {
		holder, ok := g.holderFor(in)
		if !ok {
			return refuse("здесь об этом не расскажут"), true
		}
		// Банк тем строится из party_knowledge: спросить о неизвестном нельзя —
		// кроме mandatory-фактов, которые выдаются независимо от того, знает
		// ли парти об их существовании заранее.
		if !holder.Mandatory && !g.K.Knows(in.Args.Topic) {
			return refuse("парти об этом ничего не знает — спрашивать не о чем"), true
		}
	}
	return TurnResult{}, false
}

// holderFor находит держателя, который может выдать запрошенный факт именно
// этим глаголом при выполненных требованиях.
//
// Если тема задана (question/ask_about/cross_reference), путь topic-driven
// не меняется: это единственная защита от угадывания — спросить можно
// только о том, что уже в party_knowledge (проверяется в validate).
//
// Если темы нет (examine/search/stake_out/tail), глагол не спрашивает о
// заранее известном — он ОБНАРУЖИВАЕТ факты у цели: перебираются все holders
// в базе, чей HolderID совпадает с целью, и берётся первый, для которого
// разрешён глагол, выполнены requires, снят латентный запрет и факт ещё не
// известен парти. Порядок перебора детерминирован (сортировка по FactID),
// поэтому повторные examine выдают факты в стабильной, предсказуемой
// последовательности.
func (g *Game) holderFor(in Intent) (store.FactHolder, bool) {
	if in.Args.Topic != "" {
		for _, h := range g.DB.HoldersOf(in.Args.Topic) {
			if h.HolderID != in.Args.Target {
				continue
			}
			if !gateAllows(h.Gate, in.Verb) {
				continue
			}
			if !g.requirementsMet(h.Gate) {
				continue
			}
			if h.Latent && !g.Unlocked("topic", string(h.FactID)) {
				continue
			}
			return h, true
		}
		return store.FactHolder{}, false
	}

	if in.Args.Target == "" {
		return store.FactHolder{}, false
	}

	factIDs := make([]store.FactID, 0, len(g.DB.Holders))
	for fid := range g.DB.Holders {
		factIDs = append(factIDs, fid)
	}
	sort.Slice(factIDs, func(i, j int) bool { return factIDs[i] < factIDs[j] })

	for _, fid := range factIDs {
		for _, h := range g.DB.Holders[fid] {
			if h.HolderID != in.Args.Target {
				continue
			}
			if !gateAllows(h.Gate, in.Verb) {
				continue
			}
			if !g.requirementsMet(h.Gate) {
				continue
			}
			if h.Latent && !g.Unlocked("topic", string(h.FactID)) {
				continue
			}
			if g.K.Knows(h.FactID) {
				continue
			}
			return h, true
		}
	}
	return store.FactHolder{}, false
}

func gateAllows(gate store.Gate, v Verb) bool {
	for _, gv := range gate.Verbs {
		if Verb(gv) == v {
			return true
		}
	}
	return false
}

func (g *Game) requirementsMet(gate store.Gate) bool {
	return gate.Requires.Satisfied(func(f store.FactID) bool { return g.K.Knows(f) })
}

// SceneView собирает срез сцены для правил. Здесь нет и не может быть графа
// фактов и truth: типа SceneView для них просто нет полей.
func (g *Game) SceneView(in Intent) SceneView {
	ch := g.DB.Characters[g.Actor]
	view := SceneView{
		Node:       g.Node,
		NodeTags:   g.nodeTags(g.Node),
		Allies:     1,
		Foes:       g.hostileCount(),
		Undetected: !g.Detected,
		ActorTier:  1,
		TargetTier: 1,
	}
	if ch != nil {
		view.Harm, view.Grit, view.Sheet = ch.Harm, ch.Grit, ch.Sheet
	}
	if g.tool != "" && g.toolNode == g.Node {
		view.Tools = []string{string(g.tool)}
	}
	if h, ok := g.holderFor(in); ok {
		view.GateThreshold = h.Gate.Threshold
	}
	return view
}

// isProp сообщает, что цель — инертная деталь текущего узла. Проп законная
// цель любой пробы и никогда не держит факта: холдера у него нет по типу.
func (g *Game) isProp(target store.EntityID) bool {
	_, ok := g.DB.PropAt(g.Node, store.PropID(target))
	return ok
}

func hasTag(p store.SceneProp, tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func (g *Game) nodeTags(n store.NodeID) []string {
	return g.DB.Locations[n].Tags
}

func (g *Game) hostileCount() int {
	n := 0
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if g.D.Disposition(e.ID) <= -2 {
			n++
		}
	}
	return n
}

// flavourKeyFor выбирает ключ флейвора для результата хода. Если держатель
// найден и участвует в выдаче факта, ключ строится из фактического
// держателя и факта — так у каждого выданного факта своя проза, даже когда
// глагол не задавал тему явно (examine/search/stake_out/tail). Если
// держателя нет (look, move_zone, theorize и т.п.), используется прежний
// откат на verb.target/verb.node.
func (g *Game) flavourKeyFor(in Intent, holder store.FactHolder, found bool) string {
	verb := string(in.Verb)
	// У пропа один текст на все пробы. Так и задумано: проп инертен, и разная
	// проза на look и examine намекала бы, что в нём что-то есть.
	if g.isProp(in.Args.Target) {
		return "prop." + string(in.Args.Target)
	}
	if found {
		return g.resolveFlavour(
			verb+"."+string(holder.HolderID)+"."+string(holder.FactID),
			verb+"."+string(holder.FactID),
			verb+"."+string(holder.HolderID),
			verb,
		)
	}
	return g.flavourKey(in)
}

// flavourKey подбирает ключ по цепочке от частного к общему. Общий ключ на
// глагол позволяет написать текст для talk_to один раз, а не на каждую пару
// глагол-сущность: soft-глаголы фактов не выдают, и частная проза им нужна
// не всегда. Без цепочки такой глагол показывал игроку [ключ].
func (g *Game) flavourKey(in Intent) string {
	verb := string(in.Verb)
	switch {
	case in.Args.Topic != "":
		return g.resolveFlavour(
			verb+"."+string(in.Args.Target)+"."+string(in.Args.Topic),
			verb+"."+string(in.Args.Topic),
			verb+"."+string(in.Args.Target),
			verb,
		)
	case in.Args.Target != "":
		return g.resolveFlavour(verb+"."+string(in.Args.Target), verb)
	case in.Args.Node != "":
		// Цель перемещения важнее места, откуда уходят.
		return g.resolveFlavour(verb+"."+string(in.Args.Node), verb+"."+string(g.Node), verb)
	default:
		return g.resolveFlavour(verb+"."+string(g.Node), verb)
	}
}

// resolveFlavour возвращает первый ключ, для которого есть текст. Если текста
// нет ни для одного, возвращается САМЫЙ ЧАСТНЫЙ — тогда в выводе появляется
// именно тот [ключ], который автору дела и надо написать.
func (g *Game) resolveFlavour(candidates ...string) string {
	for _, k := range candidates {
		if _, ok := g.flavour[k]; ok {
			return k
		}
	}
	return candidates[0]
}

func refuse(msg string) TurnResult { return TurnResult{Refused: true, Refusal: msg} }
