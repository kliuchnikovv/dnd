package threshold

import "github.com/kliuchnikovv/dnd/core"

// System реализует core.RuleSystem. Resolve — чистая функция от
// (Intent, SceneView, Dice): никакого состояния между вызовами.
type System struct{}

func New() *System { return &System{} }

func (s *System) Resolve(in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
	def := core.Verbs[in.Verb]

	// Глагол без броска просто удаётся. Кость не трогаем: сдвиг стрима здесь
	// сломал бы воспроизводимость прогона по seed.
	//
	// Безопасный переход сюда же. «Ты не дошёл до соседнего дома» — не выбор и
	// не цена, а сбой: игрок остаётся там, где не собирался, и всё, что он
	// планировал дальше, разъезжается. Под наблюдением враждебных переход
	// снова становится действием и снова бросается.
	if !def.Rolls || (in.Verb == "move_zone" && view.Foes == 0) {
		return core.Resolution{Class: core.OutcomeSuccess, Margin: 0}
	}

	sheet, _ := ParseSheet(view.Sheet)
	attr := sheet.Attr(in.Verb)
	tag := sheet.TagBonus(in.Verb, view)
	sit := Situational(in, view)
	th := ThresholdFor(view.GateThreshold)

	// push — заявка игрока: одно очко grit за +2. Решение принимается до
	// броска, и в этом вся его цена.
	push := 0
	if in.Push && view.Grit > 0 {
		push = PushValue
	}

	die := d.Roll(1, 20)
	total := die + attr + tag + sit + push
	margin := total - th
	class := classify(margin)

	switch die {
	case 20:
		class = class.Up()
	case 1:
		class = class.Down()
	}

	res := core.Resolution{
		Class:  class,
		Margin: margin,
		Log: core.RollLog{
			Die: die, Total: total, Threshold: th,
			Terms: []core.RollTerm{
				{Name: "атрибут", Value: attr},
				{Name: "тег", Value: tag},
				{Name: "ситуация", Value: sit},
				{Name: "push", Value: push},
			},
		},
	}

	if push > 0 {
		res.Mutations = append(res.Mutations,
			core.Mutation{Kind: core.MutResource, Target: "grit", Delta: -1})
	}

	res.Costs = costFor(def.Class, res.Class, res.Margin)
	return res
}

func classify(margin int) core.Outcome {
	switch {
	case margin >= 5:
		return core.OutcomeCrit
	case margin >= 0:
		return core.OutcomeSuccess
	case margin >= -4:
		return core.OutcomePartial
	default:
		return core.OutcomeFail
	}
}
