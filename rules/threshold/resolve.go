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
	// Переход сюда же. «Ты не дошёл до соседнего дома» — не выбор и не цена, а
	// сбой: игрок остаётся там, где не собирался, и всё, что он планировал
	// дальше, разъезжается.
	//
	// Недружелюбный свидетель — не противодействие. В M1a переходу физически
	// мешать некому: боя нет, погони нет. Уход из-под опасности — это flee, и
	// он бросается по-прежнему.
	if !def.Rolls || in.Verb == "move_zone" {
		return core.Resolution{Class: core.OutcomeSuccess, Margin: 0}
	}

	sheet, _ := ParseSheet(view.Sheet)
	attr := sheet.Attr(in.Verb)
	tag := sheet.TagBonus(in.Verb, view)
	sit := Situational(in, view)
	th := ThresholdFor(DifficultyFor(in, view))
	adv := Vantage(in, view)

	// push — заявка игрока: одно очко grit за +2. Решение принимается до
	// броска, и в этом вся его цена.
	push := 0
	if in.Push && view.Grit > 0 {
		push = PushValue
	}

	// Обстановка — преимуществом/помехой: 2d20 бери больший/меньший (D&D adv/dis).
	// Нат-1 и нат-20 на проверках характеристик НЕ особые (RAW; прото §10): класс
	// определяется только маржой — крит по марже ≥+5, сетбэк по марже ≤−10 (в cost).
	die := rollVantage(d, adv)
	total := die + attr + tag + sit + push
	margin := total - th
	class := classify(margin)

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

// rollVantage кидает честный d20 с преимуществом/помехой: adv>0 — 2d20 берём
// больший, adv<0 — меньший, 0 — один бросок (второй d20 НЕ тратится, чтобы не
// сдвигать seed-поток на ровных проверках).
func rollVantage(d core.Dice, adv int) int {
	die := d.Roll(1, 20)
	if adv == 0 {
		return die
	}
	other := d.Roll(1, 20)
	if (adv > 0 && other > die) || (adv < 0 && other < die) {
		return other
	}
	return die
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

func init() {
	core.RegisterRuleset(core.RulesetThreshold, func() core.RuleSystem { return New() })
}
