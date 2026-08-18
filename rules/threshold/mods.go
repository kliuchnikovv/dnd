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
// -2 однократно, а не -2 за каждый.
var adverseTags = []string{"dark", "rain", "crowd"}

// Situational вычисляется, не оценивается: ни одного «на усмотрение мастера».
// Сумма клампится в ±SituationalCap.
func Situational(in core.Intent, view core.SceneView) int {
	sum := 0

	// Численное превосходство учитывается только при реальном противостоянии:
	// без противников (Foes == 0) бонус/штраф большинства не применяется.
	if view.Foes > 0 {
		switch {
		case view.Allies > view.Foes:
			sum += 2
		case view.Allies < view.Foes:
			sum -= 2
		}
	}
	if view.Cover {
		sum += 2
	}
	if view.Undetected {
		sum += 2
	}
	for _, tag := range adverseTags {
		if view.HasTag(tag) {
			sum -= 2
			break
		}
	}
	sum -= 2 * view.Harm
	switch {
	case view.ActorTier > view.TargetTier:
		sum += 2
	case view.ActorTier < view.TargetTier:
		sum -= 2
	}
	if in.Args.Item != "" && view.HasTool(in.Args.Item) {
		sum += 2
	}

	if sum > SituationalCap {
		return SituationalCap
	}
	if sum < -SituationalCap {
		return -SituationalCap
	}
	return sum
}
