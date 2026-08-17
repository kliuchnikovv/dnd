package threshold

import "github.com/kliuchnikovv/dnd/core"

// CatastrophicMargin — маржа, начиная с которой цена провала удваивается.
const CatastrophicMargin = -10

// costTable — выбор цены из таксономии ядра по классу глагола. Система правил
// только выбирает; исполняет ядро. Строка reason пуста намеренно: рассуждение
// не имеет цены.
var costTable = map[core.VerbClass]map[core.Outcome][]core.CostKind{
	core.ClassInvestigate: {
		core.OutcomePartial: {core.CostTickClock},
		core.OutcomeFail:    {core.CostFalseLead},
	},
	core.ClassReason: {},
	core.ClassSocial: {
		core.OutcomePartial: {core.CostDebt},
		core.OutcomeFail:    {core.CostTickClock, core.CostDispositionDown},
	},
	core.ClassMove: {
		core.OutcomePartial: {core.CostPositionWorse},
		core.OutcomeFail:    {core.CostTickClock},
	},
	core.ClassAttack: {
		core.OutcomePartial: {core.CostTickClock},
		core.OutcomeFail:    {core.CostHarmSelf},
	},
	core.ClassSupport: {
		core.OutcomePartial: {core.CostHalfEffect},
		core.OutcomeFail:    {core.CostHarmSelf},
	},
	core.ClassResource: {
		core.OutcomePartial: {core.CostResourceSpent},
		core.OutcomeFail:    {core.CostResourceSpent},
	},
	core.ClassSkill: {
		core.OutcomePartial: {core.CostPositionWorse},
		core.OutcomeFail:    {core.CostTickClock, core.CostPositionWorse},
	},
}

func costFor(class core.VerbClass, out core.Outcome, margin int) []core.CostKind {
	if out != core.OutcomePartial && out != core.OutcomeFail {
		return nil
	}
	base := costTable[class][out]
	if len(base) == 0 {
		return nil
	}
	out2 := make([]core.CostKind, 0, 2*len(base))
	out2 = append(out2, base...)
	if margin <= CatastrophicMargin {
		out2 = append(out2, base...)
	}
	return out2
}
