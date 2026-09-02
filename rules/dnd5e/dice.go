package dnd5e

import (
	"fmt"
	"strconv"
	"strings"
)

// DiceSpec — разобранная dice-строка «NdM(+mod|+ability)».
type DiceSpec struct {
	N, Sides   int
	Mod        int
	AbilityMod string // "" | "str" | "dex" | ...
}

// ParseDice разбирает dice-строку вида "NdM", "NdM+K", "NdM+ability".
func ParseDice(s string) (DiceSpec, error) {
	s = strings.ReplaceAll(strings.TrimSpace(strings.ToLower(s)), " ", "")
	var d DiceSpec
	// Разделить по «+»: [dice, [mod]]
	parts := strings.SplitN(s, "+", 2)
	if _, err := fmt.Sscanf(parts[0], "%dd%d", &d.N, &d.Sides); err != nil {
		return d, fmt.Errorf("dnd5e/dice: не разобрал %q: %w", s, err)
	}
	if len(parts) == 2 {
		if n, err := strconv.Atoi(parts[1]); err == nil {
			d.Mod = n
		} else {
			switch parts[1] {
			case "str", "dex", "con", "int", "wis", "cha":
				d.AbilityMod = parts[1]
			default:
				return d, fmt.Errorf("dnd5e/dice: неизвестный мод %q", parts[1])
			}
		}
	}
	return d, nil
}
