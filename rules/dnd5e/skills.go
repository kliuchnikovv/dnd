package dnd5e

// skillAbility — какая ability стоит за skill в 5e. Таблица закрыта:
// неизвестный skill возвращает "", а не «что-нибудь похожее» — это
// значит «нет проверки», и вызывающий делает fallback на общую ability.
var skillAbility = map[string]string{
	// Str
	"athletics": "str",
	// Dex
	"acrobatics": "dex", "stealth": "dex", "sleight_of_hand": "dex",
	"thieves_tools": "dex",
	// Wis
	"perception": "wis", "insight": "wis", "medicine": "wis",
	"survival": "wis", "animal_handling": "wis",
	// Int
	"arcana": "int", "history": "int", "investigation": "int",
	"nature": "int", "religion": "int",
	// Cha
	"persuasion": "cha", "deception": "cha", "intimidation": "cha",
	"performance": "cha",
}

// SkillAbility возвращает ability, связанную с данным skill.
// Для неизвестного skill возвращает пустую строку.
func SkillAbility(skill string) string {
	return skillAbility[skill]
}
