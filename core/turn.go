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

	// Шаг 1: валидация.
	if r, bad := g.validate(in, def); bad {
		return r
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
	if in.Args.Target != "" {
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
	for _, r := range gate.Requires {
		if !g.K.Knows(r) {
			return false
		}
	}
	return true
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
	if h, ok := g.holderFor(in); ok {
		view.GateThreshold = h.Gate.Threshold
	}
	return view
}

func (g *Game) nodeTags(n store.NodeID) []string {
	return g.DB.Locations[n].Tags
}

func (g *Game) hostileCount() int {
	n := 0
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if g.Disposition[e.ID] <= -2 {
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
	if found {
		return string(in.Verb) + "." + string(holder.HolderID) + "." + string(holder.FactID)
	}
	return g.flavourKey(in)
}

func (g *Game) flavourKey(in Intent) string {
	if in.Args.Topic != "" {
		return string(in.Verb) + "." + string(in.Args.Target) + "." + string(in.Args.Topic)
	}
	if in.Args.Target != "" {
		return string(in.Verb) + "." + string(in.Args.Target)
	}
	return string(in.Verb) + "." + string(g.Node)
}

func refuse(msg string) TurnResult { return TurnResult{Refused: true, Refusal: msg} }
