package cyberpunk

import "github.com/kliuchnikovv/dnd/core"

// Verbs — глаголы, специфичные для киберпанк-рулсета. Общие (talk_to,
// examine, sneak, attack) — из core.Verbs и dnd5e-ветки: сценарий-архетип
// (adventure) уже проносит их в реестр.
//
// hack — влом системы «на месте» (терминал, дверь): TECH + interface + 1d10.
// jack_in — подключение к NET-архитектуре: короткий «войти», без развёрнутой
// NET-игры (фаза B). netrun — обобщённый ход внутри NET (в фазе A — метка
// для внешнего резолвера, без под-цикла NET).
func Verbs() []core.VerbDef {
	return []core.VerbDef{
		{Verb: "hack", Class: core.ClassSkill, Rolls: true, Hard: true, Spends: false},
		{Verb: "jack_in", Class: core.ClassResource, Rolls: false, Hard: false, Spends: false},
		{Verb: "netrun", Class: core.ClassSkill, Rolls: true, Hard: true, Spends: false},
	}
}
