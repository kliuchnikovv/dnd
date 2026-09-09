package cyberpunk

import (
	"fmt"
	"strings"
)

// DiceSpec — разобранная dice-строка вида «NdM» (RED: 2d6, 3d6, 5d6...).
// В отличие от dnd5e, никаких AbilityMod: урон в RED — чистая кость оружия.
type DiceSpec struct {
	N, Sides int
}

// ParseDice разбирает строку «NdM». Пустая строка возвращает нулевой спек
// без ошибки: это «не-оружие» (для тестов и пустых листов).
func ParseDice(s string) (DiceSpec, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return DiceSpec{}, nil
	}
	var d DiceSpec
	if _, err := fmt.Sscanf(s, "%dd%d", &d.N, &d.Sides); err != nil {
		return d, fmt.Errorf("cyberpunk/dice: не разобрал %q: %w", s, err)
	}
	return d, nil
}
