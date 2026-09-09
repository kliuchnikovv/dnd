package view

import "github.com/kliuchnikovv/dnd/core"

// Реестр view-правил: имя правила (core.RulesetKind) → фабрика презентации
// (view.Ruleset). Аналог core.RegisterRuleset, только по view-оси: механику
// правила регистрирует core, а его меры — эта таблица. Здесь ЖИВЁТ ТОЛЬКО
// интерфейс view.Ruleset; слов конкретного правила (grit, Humanity, часы) сам
// view знать не должен — их приносят реализации из своих пакетов-презентаций
// side-effect'ом импорта. Такой шов оставляет view агностичным, а список
// известных правил — открытым.
var rulesetRegistry = map[core.RulesetKind]func() Ruleset{}

// RegisterRuleset — зарегистрировать view-фабрику правила. Регистрация одна на
// процесс: init-функция пакета-презентации зовёт её при импорте.
func RegisterRuleset(k core.RulesetKind, factory func() Ruleset) {
	rulesetRegistry[k] = factory
}

// LookupRuleset — получить view-правило по имени. Второй результат ложен, если
// правило не зарегистрировано (пакет-презентация не импортирован).
func LookupRuleset(k core.RulesetKind) (Ruleset, bool) {
	f, ok := rulesetRegistry[k]
	if !ok {
		return nil, false
	}
	return f(), true
}
