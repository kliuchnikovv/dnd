package threshold

import "github.com/kliuchnikovv/dnd/core"

// Каноничная D&D-шкала DC (SRD: Ability Checks): very_easy 5 · easy 10 ·
// medium 15 · hard 20 · very_hard 25 · extreme 30. Автор-банды дела
// (easy/normal/hard) ложатся на 10/15/20; крайние ступени — для судьи и
// генератора.
const (
	ThresholdVeryEasy = 5
	ThresholdEasy     = 10
	ThresholdNormal   = 15 // = medium
	ThresholdHard     = 20
	ThresholdVeryHard = 25
	ThresholdExtreme  = 30

	TagValue       = 2
	PushValue      = 2
	SituationalCap = 4
)

// ThresholdFor переводит сложность из данных дела в число по каноничной шкале.
// Множество закрыто: всё незнакомое — «medium» (15).
func ThresholdFor(gate string) int {
	switch gate {
	case "very_easy", "trivial":
		return ThresholdVeryEasy
	case "easy":
		return ThresholdEasy
	case "hard":
		return ThresholdHard
	case "very_hard":
		return ThresholdVeryHard
	case "extreme", "impossible", "nearly_impossible":
		return ThresholdExtreme
	default: // normal, medium, "", неизвестное
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

// FactorNames — все ситуативные (АДДИТИВНЫЕ) факторы в стабильном порядке.
// Существует ради теста частоты срабатывания. Обстановка (adverse-среда, инструмент)
// сюда больше не входит: она ушла в advantage/disadvantage (см. Vantage) — по D&D
// помехи среды идут через 2d20, а не через слагаемое к броску.
func FactorNames() []string {
	return []string{"numbers", "cover", "undetected", "harm", "tier"}
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

	// Обстановка (adverse-среда) и инструмент, её отменяющий, здесь больше НЕ
	// учитываются: они ушли в advantage/disadvantage (см. Vantage). По D&D помеха
	// среды — это 2d20-помеха, а не -2 к сумме, иначе за один и тот же факт платили
	// бы дважды (и слагаемым, и ступенью).

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
