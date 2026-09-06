package threshold

import "github.com/kliuchnikovv/dnd/core"

// Vantage — преимущество (+1) / помеха (-1) / ровно (0) от ОБСТАНОВКИ. По D&D
// (SRD: Ability Scores) и прото §6: сложность задачи — это DC, а помехи и подмога
// окружения идут через advantage/disadvantage (2d20 бери больше/меньше), НЕ через
// кручение порога. Не стакается: много источников — всё равно одна ступень;
// встречные (открытая цель И сопротивление) гасят друг друга.
//
// Источники помехи: сопротивляющаяся цель (Opposed) и adverse-среда (тьма/дождь/
// толпа/шум) — но инструмент (фонарь и т.п.) отменяет именно помеху среды, как
// раньше отменял её ситуативное слагаемое. Источник преимущества: открытая,
// незащищённая цель (Exposed).
func Vantage(_ core.Intent, view core.SceneView) int {
	adv := view.Exposed

	adverse := false
	for _, tag := range adverseTags {
		if view.HasTag(tag) {
			adverse = true
			break
		}
	}
	dis := view.Opposed || (adverse && len(view.Tools) == 0)

	switch {
	case adv == dis: // оба или ни одного — ровно
		return 0
	case adv:
		return +1
	default:
		return -1
	}
}
