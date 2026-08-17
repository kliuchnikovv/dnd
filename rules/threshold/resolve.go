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
	if !def.Rolls {
		return core.Resolution{Class: core.OutcomeSuccess, Margin: 0}
	}

	sheet, _ := ParseSheet(view.Sheet)
	attr := sheet.Attr(in.Verb)
	tag := sheet.TagBonus(in.Verb, view)
	sit := Situational(in, view)
	th := ThresholdFor(view.GateThreshold)

	die := d.D20()
	total := die + attr + tag + sit
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
			},
		},
	}

	// grit конвертирует выпавший Провал в Частично, тратя одно очко.
	if res.Class == core.OutcomeFail && view.Grit > 0 {
		res.Class = core.OutcomePartial
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
