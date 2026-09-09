package cyberpunk

import (
	"github.com/kliuchnikovv/dnd/core"
)

// System реализует core.RuleSystem для Cyberpunk RED (фаза A).
// Никакого состояния между вызовами: сессионные меры (HP, Humanity, LUCK)
// живут в листе и store, а не в резолвере.
type System struct{}

func New() *System { return &System{} }

// verbStat — какая STAT/SKILL проверяет глагол по умолчанию. Таблица
// закрыта: расширять словарь — задача данных дела (case.json → checks:),
// а не резолвера. Пустое имя навыка — «голый STAT-бросок»: RED разрешает.
type verbSpec struct {
	stat, skill string
}

var verbSpecs = map[core.Verb]verbSpec{
	// Общие
	"sneak":    {StatDEX, "stealth"},
	"hide":     {StatDEX, "stealth"},
	"examine":  {StatINT, "perception"},
	"search":   {StatINT, "perception"},
	"question": {StatEMP, "human_perception"},
	"persuade": {StatCOOL, "persuasion"},
	// Киберпанк-глаголы
	"hack":   {StatTECH, "interface"},
	"netrun": {StatINT, "interface"},
}

// Resolve — единственная точка обращения ядра к правилам. Диспатчит по
// глаголу: attack идёт в attackResolve (нужен вес оружия), остальные — в
// checkResolve. Глагол без броска (Rolls=false) успевает без проверки: то
// же поведение, что у threshold/dnd5e.
func (s *System) Resolve(in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
	def := core.Verbs[in.Verb]
	if !def.Rolls || in.Verb == "move_zone" {
		return core.Resolution{Class: core.OutcomeSuccess}
	}

	sheet, _ := ParseSheet(view.Sheet)
	wound := woundPenalty(view.Harm, sheet.MaxHP())

	if def.Class == core.ClassAttack {
		return s.resolveAttack(sheet, in, view, d, wound)
	}
	spec, ok := verbSpecs[in.Verb]
	if !ok {
		spec = verbSpec{StatINT, ""} // незнакомый глагол — голый d10+INT
	}
	dv := DVOfIntent(in, view)
	roll1 := d.Roll(1, 10)
	var roll2 int
	if roll1 == 10 || roll1 == 1 {
		roll2 = d.Roll(1, 10)
	}
	return check(sheet, spec.stat, spec.skill, dv, wound, roll1, roll2)
}

// resolveAttack — атака первым оружием листа. Выбор оружия из нескольких —
// задача авторства кейса (фаза A: первое, как у dnd5e).
func (s *System) resolveAttack(sheet Sheet, in core.Intent, view core.SceneView, d core.Dice, wound int) core.Resolution {
	if len(sheet.Weapons) == 0 {
		return core.Resolution{Class: core.OutcomeFail}
	}
	w := sheet.Weapons[0]
	dmgSpec, _ := ParseDice(w.Damage)
	roll1 := d.Roll(1, 10)
	var roll2 int
	if roll1 == 10 || roll1 == 1 {
		roll2 = d.Roll(1, 10)
	}
	dmg := 0
	if dmgSpec.N > 0 {
		dmg = d.Roll(dmgSpec.N, dmgSpec.Sides)
	}
	// DV атаки: если авторство кейса проставило SceneView.DC — используем;
	// иначе Difficult (обыденная средняя дальность в RED).
	dv := view.DC
	if dv == 0 {
		dv = DVDifficult
	}
	// SP цели: в фазе A нет отдельного SP-поля в SceneView, поэтому
	// используем TargetAC как канал: авторство кейса кладёт туда SP в
	// точках броневых сущностей. Это временное имя канала; фаза B добавит
	// SceneView.TargetSP отдельным полем.
	res := attack(sheet, w, dv, view.TargetAC, wound, roll1, roll2, dmg)
	if res.Damage > 0 && in.Args.Target != "" {
		res.Mutations = append(res.Mutations, core.Mutation{
			Kind: core.MutHPDelta, Target: string(in.Args.Target), Amount: -res.Damage,
		})
	}
	return res
}

// woundPenalty — штраф проверок по статусу ран. RED:
// Seriously Wounded (HP ≤ ½ max) → −2; Mortally Wounded (HP < 0) → −4.
// В SceneView живёт заполненная ячейка Harm — но у cyberpunk другая шкала
// (HP из листа). В фазе A принимаем: Harm интерпретируется как «текущий
// урон» (max−hp) — авторство кейса может подставить его, иначе 0 = здоров.
func woundPenalty(harm, maxHP int) int {
	if maxHP <= 0 {
		return 0
	}
	hp := maxHP - harm
	if hp < 0 {
		return -4
	}
	if SeriouslyWounded(hp, maxHP) {
		return -2
	}
	return 0
}

// DVOfIntent — сложность для не-атаки: явный SceneView.DC приоритетнее,
// затем CheckDC (данные "checks:"), потом DefaultDV.
func DVOfIntent(in core.Intent, view core.SceneView) int {
	if view.DC != 0 {
		return view.DC
	}
	if dc := view.CheckDC(string(in.Verb), string(in.Args.Target)); dc > 0 {
		return dc
	}
	return DefaultDV
}

func init() {
	core.RegisterRuleset(core.RulesetCyberpunk, func() core.RuleSystem { return New() })
	// Регистрируем киберпанк-глаголы в общий реестр core.Verbs напрямую,
	// а не через Scenario.ExtraVerbs: адвенчурный сценарий уже несёт свою
	// пачку (dnd5e.Verbs), а плодить новый архетип ради трёх глаголов было
	// бы дороже, чем сделать их частью словаря процесса. Реестр — глобальная
	// карта; неиспользуемые глаголы в аффордансах не всплывают.
	for _, v := range Verbs() {
		core.Verbs[v.Verb] = v
	}
}
