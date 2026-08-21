package threshold

import "github.com/kliuchnikovv/dnd/core"

const (
	ThresholdEasy   = 10
	ThresholdNormal = 14
	ThresholdHard   = 18

	TagValue       = 2
	SituationalCap = 4
)

// ThresholdFor переводит сложность из данных дела в число. Множество закрыто:
// всё незнакомое — «Норма».
func ThresholdFor(gate string) int {
	switch gate {
	case "easy":
		return ThresholdEasy
	case "hard":
		return ThresholdHard
	default:
		return ThresholdNormal
	}
}

// adverseTags — обстановка, мешающая действию. Присутствие любого из них даёт
// -2 однократно, а не -2 за каждый. Теги ставит автор УЗЛА: среда — свойство
// места. Проп отрицательного тега не несёт, иначе нарратор, расставляя детали,
// двигал бы матожидание броска.
var adverseTags = []string{"dark", "rain", "crowd", "noise"}

// stealthVerbs — глаголы, для которых незаметность вообще имеет смысл.
// Расспрашивать дружелюбного свидетеля незаметно нельзя как понятие, а не
// только арифметически.
var stealthVerbs = map[core.Verb]bool{
	"sneak": true, "tail": true, "stake_out": true, "pick": true,
	"move_zone": true, "flee": true, "strike": true,
}

// FactorNames — все ситуативные факторы в стабильном порядке. Существует ради
// теста частоты срабатывания: фактор, который срабатывает почти всегда, — это
// неверно назначенный порог, и найти такой надо до, а не после.
func FactorNames() []string {
	return []string{"numbers", "cover", "undetected", "adverse", "harm", "tier", "tool"}
}

// SituationalFactors раскладывает ситуативные модификаторы по факторам, до
// клампа. Situational — их сумма; отдельный разбор нужен, чтобы частоту
// каждого можно было измерить, а не оценить на глаз.
func SituationalFactors(in core.Intent, view core.SceneView) map[string]int {
	f := map[string]int{}

	// Численное превосходство учитывается только при реальном противостоянии:
	// без противников бонус/штраф большинства не применяется.
	if view.Foes > 0 {
		switch {
		case view.Allies > view.Foes:
			f["numbers"] = 2
		case view.Allies < view.Foes:
			f["numbers"] = -2
		}
	}
	if view.Cover {
		f["cover"] = 2
	}

	// «Не обнаружен» — не базовая линия, а достижение: +2 положен только там,
	// где незаметность что-то значит и есть от кого прятаться. Модификатор,
	// висящий по умолчанию, не несёт информации.
	if view.Undetected && stealthVerbs[in.Verb] && view.Foes > 0 {
		f["undetected"] = 2
	}

	for _, tag := range adverseTags {
		if view.HasTag(tag) {
			f["adverse"] = -2
			break
		}
	}
	// Инструмент не поднимает базу, а отменяет штраф среды: фонарь в тёмном
	// подвале возвращает к норме, а не делает лучше нормы. Он стоит игроку
	// хода, и в этом вся плата за него.
	if f["adverse"] < 0 && len(view.Tools) > 0 {
		f["tool"] = -f["adverse"]
	}

	if view.Harm > 0 {
		f["harm"] = -2 * view.Harm
	}
	switch {
	case view.ActorTier > view.TargetTier:
		f["tier"] = 2
	case view.ActorTier < view.TargetTier:
		f["tier"] = -2
	}
	return f
}

// Situational вычисляется, не оценивается: ни одного «на усмотрение мастера».
// Сумма клампится в ±SituationalCap.
func Situational(in core.Intent, view core.SceneView) int {
	sum := 0
	for _, v := range SituationalFactors(in, view) {
		sum += v
	}
	if sum > SituationalCap {
		return SituationalCap
	}
	if sum < -SituationalCap {
		return -SituationalCap
	}
	return sum
}
