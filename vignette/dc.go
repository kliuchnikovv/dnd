package vignette

// Каноничная D&D-шкала сложности (SRD: Ability Checks), СОБСТВЕННАЯ у виньетки.
// Движок самодостаточен (ADR-0009): свою DC-шкалу он не заимствует у M1a-рулсета
// rules/threshold — тот измерительный инструмент мы не трогаем, а его калибровка
// (10/14/18) не должна протекать в хоррор-механику. Из ядра берётся только
// честная кость (core.Dice).
const (
	DCVeryEasy = 5
	DCEasy     = 10
	DCMedium   = 15
	DCHard     = 20
	DCVeryHard = 25
	DCExtreme  = 30
)

// DCFromBand переводит band судьи в число по каноничной шкале. Всё незнакомое —
// medium (15).
func DCFromBand(band string) int {
	switch band {
	case "very_easy", "trivial":
		return DCVeryEasy
	case "easy":
		return DCEasy
	case "hard":
		return DCHard
	case "very_hard":
		return DCVeryHard
	case "extreme", "impossible", "nearly_impossible":
		return DCExtreme
	default: // medium, normal, "", неизвестное
		return DCMedium
	}
}
