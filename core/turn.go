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
	// SpokenBy — кто произносит выданный факт вслух. Пусто, когда произносить
	// некому: факт достался от вещи, от отсутствующего или не достался вовсе.
	//
	// Решает это ЯДРО, и другого места решения нет. Раньше «кто говорит»
	// определяли два слоя над доменом, каждый по признаку «в ходу есть
	// Learned»: презентация глушила подпись, актёр глушил сам себя. Признак
	// слишком грубый — труп не говорит, а стражник говорит, — а два места
	// правды об одном расходятся молча.
	SpokenBy store.EntityID
}

// Check отвечает на вопрос «мир это примет?», НИЧЕГО НЕ МЕНЯЯ.
//
// Нужен чат-режиму: там реплика Мастера показывается ДО того, как ход
// исполнен, и показать её на действие, которое ядро не пропустит, значит
// соврать игроку голосом мира. Спросить вместо этого Apply нельзя — он
// тратит ход: тикает часы и считает холостые ходы, и ход посчитался бы дважды.
//
// Отказывает Check ровно там же и теми же словами, что Apply, — потому что
// Apply ходит через него же: второго места правды о том, что миру можно, не
// появляется. Тест прогоняет каждый отказ через обе двери на случай, если
// когда-нибудь появится.
func (g *Game) Check(in Intent) TurnResult {
	def, ok := Verbs[in.Verb]
	if !ok {
		return refuse("неизвестное действие")
	}
	// theorize мир не проверяет: это заметка игрока, а не действие в нём.
	// Пустой её не бывает — записывать нечего.
	if in.Verb == "theorize" {
		if in.Args.Text == "" {
			return refuse("гипотеза не может быть пустой")
		}
		return TurnResult{}
	}
	if def.Hard && g.Incapacitated() {
		return refuse("персонаж выведен из строя — сначала отдых")
	}
	if r, bad := g.validate(in, def); bad {
		return r
	}
	return TurnResult{}
}

// Apply — пятишаговый ход: валидация, ветка без броска, сборка SceneView,
// Resolve, применение. Ядро никогда не видит кости: они уходят внутрь Resolve.
func (g *Game) Apply(in Intent) TurnResult {
	def, ok := Verbs[in.Verb]
	if !ok {
		return refuse("неизвестное действие")
	}

	// Шаг 1: валидация. Она вся живёт в Check, и Apply ходит через него же:
	// иначе о том, что миру можно, было бы два места правды, и чат-режим стал
	// бы показывать реплику Мастера на ход, который ядро не пропустит.
	if r := g.Check(in); r.Refused {
		// Отказ — самый чистый признак того, что игрок встал: он попробовал, и
		// мир не принял. Ход при этом не потрачен, часы не тикают — но
		// счётчик холостых ходов обязан отказ видеть, иначе чутьё молчит
		// именно тогда, когда нужно. Застрявший игрок производит отказы
		// дюжинами: живой плейтест так и прошёл мимо всех подсказок.
		g.noteTurn(def, 0)
		return r
	}

	// theorize — заметка игрока, а не действие в мире: кубика не бросает и
	// часов не тикает. Пустой она не бывает — это уже отсеял Check.
	if in.Verb == "theorize" {
		g.Theories = append(g.Theories, in.Args.Text)
		return TurnResult{FlavourKey: "theorize.recorded"}
	}

	// Взять инструмент — отдельный ход без броска. Инструмент ничего не даёт,
	// пока лежит или лежит в кармане: плата за отмену штрафа среды —
	// потраченное действие.
	if in.Verb == "use_item" {
		if key, ok := g.readyTool(in.Args.Item); ok {
			return TurnResult{FlavourKey: key}
		}
	}

	// Предъявление — видимое действие, и его след обязан лечь ДО поиска
	// держателя: иначе гейт на предмет не увидит того, что ему только что
	// показали, и факт откроется лишь со второго раза.
	if in.Verb == "present" {
		if r, bad := g.present(in); bad {
			return r
		}
	}

	holder, found := g.holderFor(in)

	// Шаг 2: ветка без броска.
	if !def.Rolls || (found && holder.Mandatory) {
		res := TurnResult{FlavourKey: g.flavourKeyFor(in, holder, found)}
		if found {
			if g.learn(holder.FactID, holder.HolderID) {
				res.Learned = append(res.Learned, Learned{holder.FactID, holder.HolderID})
				res.SpokenBy = g.spokenBy(holder.HolderID)
				g.applyUnlocksFor(holder.FactID)
			}
		}
		spent := g.spendTime(def)
		g.applyConsequences(spent)
		res.Fired = append(res.Fired, spent...)
		g.noteTurn(def, len(res.Learned))
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
		if g.learn(holder.FactID, holder.HolderID) {
			out.Learned = append(out.Learned, Learned{holder.FactID, holder.HolderID})
			out.SpokenBy = g.spokenBy(holder.HolderID)
			g.applyUnlocksFor(holder.FactID)
		}
	}
	// Применяется только то, что сработало ИМЕННО СЕЙЧАС: HostileTo не
	// идемпотентен, и повторный проход по уже применённому списку удвоил бы
	// сдвиг расположения молча.
	spent := g.spendTime(def)
	g.applyConsequences(spent)
	out.Fired = append(out.Fired, spent...)
	g.noteTurn(def, len(out.Learned))
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
	if in.Args.Node != "" && !g.KnowsPlace(in.Args.Node) {
		return refuse("ты не знаешь, где это"), true
	}
	for _, f := range in.Args.Facts {
		if !g.K.Knows(f) {
			return refuse("этот факт парти неизвестен"), true
		}
	}
	if in.Args.Topic != "" {
		holder, held := g.holderFor(in)
		// Отсутствие держателя — НЕ отказ: человек просто не знает, и сказать
		// ему об этом — обычный ход разговора. Раньше здесь стояла стена, и
		// живой плейтест бился в неё десятками ходов подряд.
		//
		// Бросок при этом остаётся (его делает Apply): отсутствие броска метило
		// бы незнающих — та же логика, по которой осмотр инертной детали
		// бросает кость наравне с настоящей целью.
		//
		// Банк тем строится из party_knowledge: спросить о неизвестном нельзя —
		// кроме mandatory-фактов, которые выдаются независимо от того, знает
		// ли парти об их существовании заранее.
		if !g.K.Knows(in.Args.Topic) && !(held && holder.Mandatory) {
			return refuse("парти об этом ничего не знает — спрашивать не о чем"), true
		}
	}
	return TurnResult{}, false
}

// spokenBy — кто из держателей вправе произнести факт вслух. Человек в этой же
// сцене; вещь и отсутствующий молчат.
//
// Отсутствующий молчит не из вредности: факт от него законен (его мог назвать
// кто угодно, кто держит ту же строку), но реплика из пустого места читается
// как голос ниоткуда.
func (g *Game) spokenBy(id store.EntityID) store.EntityID {
	e, ok := g.DB.Entities[id]
	if !ok || e.Kind != store.EntityNPC || e.Node != g.Node {
		return ""
	}
	return id
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
			if !g.itemsPresented(h) {
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
			if !g.itemsPresented(h) {
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

// itemsPresented — предъявлены ли этому держателю все предметы, которых требует
// гейт. Проверяется предъявление, а не владение: показать бумагу — видимое
// действие с реакцией, а бумага в кармане открывала бы двери молча.
func (g *Game) itemsPresented(h store.FactHolder) bool {
	for _, id := range h.Gate.RequiresItems {
		if !g.D.Presented(h.HolderID, id) {
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
	if g.tool != "" && g.toolNode == g.Node {
		view.Tools = []string{string(g.tool)}
	}
	if h, ok := g.holderFor(in); ok {
		view.GateThreshold = h.Gate.Threshold
	}
	view.Opposed = g.opposes(in.Args.Target)
	// Exposed остаётся false: состояния, выражающего беспомощность цели
	// (оглушена, связана, не защищается), в M1a ещё нет — боя нет. Канал
	// заведён здесь, чтобы, когда такое состояние появится, порог читал его
	// из состояния, а не из прозы Мастера.
	return view
}

// opposes сообщает, есть ли у цели воля и намерение мешать. Воля — свойство
// сущности: у пропа, записи и отсутствующего её нет по типу, а не по
// расположению. Соседи по узлу сюда не входят: численное превосходство уже
// учтено ситуативным слагаемым, и второй раз считать его нельзя.
func (g *Game) opposes(target store.EntityID) bool {
	if target == "" {
		return false
	}
	e, ok := g.DB.Entities[target]
	if !ok || e.Kind != store.EntityNPC || e.Node != g.Node {
		return false
	}
	return g.D.Disposition(target) <= hostileDisposition
}

// isProp сообщает, что цель — инертная деталь текущего узла. Проп законная
// цель любой пробы и никогда не держит факта: холдера у него нет по типу.
func (g *Game) isProp(target store.EntityID) bool {
	_, ok := g.DB.PropAt(g.Node, store.PropID(target))
	return ok
}

// spendTime списывает игровое время у глаголов, которые его тратят по своей
// природе. Отказ сюда не доходит: ход не состоялся, значит и время не ушло.
// Свободные пробы не помечены и остаются бесплатными всегда — игра,
// наказывающая за то, что игрок смотрит по сторонам, измеряет не то.
func (g *Game) spendTime(def VerbDef) []store.Consequence {
	if !def.Spends {
		return nil
	}
	return g.C.TickAll(1)
}

func (g *Game) nodeTags(n store.NodeID) []string {
	return g.DB.Locations[n].Tags
}

// hostileDisposition — расположение, с которого сущность считается
// враждебной. Одно место правды: и подсчёт противников, и признак
// противодействия обязаны проводить границу по одной черте.
const hostileDisposition = -2

func (g *Game) hostileCount() int {
	n := 0
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if g.D.Disposition(e.ID) <= hostileDisposition {
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
