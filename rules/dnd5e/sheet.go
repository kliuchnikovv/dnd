// Package dnd5e — упрощённое подмножество D&D 5e за швом core.RuleSystem.
// Лист хранит ability scores (не модификаторы) — 5e-мышление, и путь к
// левелапу открыт (см. §5 спеки, точки роста).
package dnd5e

import "encoding/json"

// Sheet — лист персонажа с характеристиками и другими данными.
type Sheet struct {
	Str   int `json:"str"`
	Dex   int `json:"dex"`
	Con   int `json:"con"`
	Int   int `json:"int"`
	Wis   int `json:"wis"`
	Cha   int `json:"cha"`
	Prof  int `json:"prof"`

	Skills []string `json:"skills"`
	Saves  []string `json:"saves"`

	MaxHP int `json:"max_hp"`
	AC    int `json:"ac"`
	Speed int `json:"speed"`

	Weapons []Weapon `json:"weapons"`
}

// Weapon — одно оружие в арсенале персонажа.
type Weapon struct {
	Name   string
	Reach  string // "melee" | "ranged"
	Attack string // "str" | "dex"
	Damage string // "1d8+str", "1d6+dex", ...
}

// ParseSheet разбирает JSON-представление листа персонажа.
func ParseSheet(raw json.RawMessage) (Sheet, error) {
	var s Sheet
	if len(raw) == 0 {
		return s, nil
	}
	return s, json.Unmarshal(raw, &s)
}

// Mod возвращает модификатор способности по строке ("str"|"dex"|...).
func (s Sheet) Mod(ability string) int {
	score := s.abilityScore(ability)
	return (score - 10) / 2
}

func (s Sheet) abilityScore(ability string) int {
	switch ability {
	case "str":
		return s.Str
	case "dex":
		return s.Dex
	case "con":
		return s.Con
	case "int":
		return s.Int
	case "wis":
		return s.Wis
	case "cha":
		return s.Cha
	}
	return 10
}

// Proficient проверяет, есть ли навык в списке профессиональных.
func (s Sheet) Proficient(skill string) bool {
	for _, x := range s.Skills {
		if x == skill {
			return true
		}
	}
	return false
}

// SaveProficient проверяет, есть ли спас в списке профессиональных.
func (s Sheet) SaveProficient(ability string) bool {
	for _, x := range s.Saves {
		if x == ability {
			return true
		}
	}
	return false
}
