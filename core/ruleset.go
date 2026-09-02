// Package core — Ruleset. Аналог Scenario для системы правил.
// Пакет регистрирует фабрики систем правил: threshold и dnd5e.
package core

type RulesetKind string

const (
	RulesetThreshold RulesetKind = "threshold"
	RulesetDND5e     RulesetKind = "dnd5e"
)

var rulesetRegistry = map[RulesetKind]func() RuleSystem{}

// RegisterRuleset — зарегистрировать фабрику системы правил.
// Регистрация одна на процесс: init-функция пакета системы правил зовёт её при импорте.
func RegisterRuleset(k RulesetKind, factory func() RuleSystem) {
	rulesetRegistry[k] = factory
}

// LookupRuleset — получить экземпляр системы правил по имени.
// Второй результат ложен, если система правил не зарегистрирована
// (например, пакет не импортирован).
func LookupRuleset(k RulesetKind) (RuleSystem, bool) {
	f, ok := rulesetRegistry[k]
	if !ok {
		return nil, false
	}
	return f(), true
}
