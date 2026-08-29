package cli

import "github.com/kliuchnikovv/dnd/core"

// Экспортируемые сборщики прозы: сервер строит аргументы Мастеру ТЕМИ ЖЕ
// helper'ами, что печатает терминал (Render.Turn/Scene/Briefing). Один
// источник правды о том, какая рамка, сцена и дайджест уходят Мастеру — иначе
// проза сервера и CLI разошлись бы молча.

// OutcomeProse — проза исхода хода. Второй результат ложен, когда описывать
// нечего: ход отказан или у исхода нет авторской рамки (FlavourKey пуст).
func OutcomeProse(g *core.Game, in core.Intent, t core.TurnResult) (Prose, bool) {
	if t.Refused || t.FlavourKey == "" {
		return Prose{}, false
	}
	return Prose{
		Kind:     ProseOutcome,
		Frame:    g.Flavour(t.FlavourKey),
		Scene:    sceneOf(g),
		Outcome:  outcomeOf(g, t),
		Speaking: speakerName(g, in, t),
		State:    core.StateDigest(g, t).Lines(),
	}, true
}

// PlaceProse — проза описания места (как Render.Scene).
func PlaceProse(g *core.Game) Prose {
	return Prose{
		Kind:  ProsePlace,
		Frame: g.Flavour("look." + string(g.Node)),
		Scene: sceneOf(g),
		State: core.StateDigest(g, core.TurnResult{}).Lines(),
	}
}

// BriefingProse — авторское введение в дело. State пуст намеренно: это
// единственное место, где игроку легально сообщают факты дела, и гвардить его
// состоянием нельзя.
func BriefingProse(g *core.Game) Prose {
	return Prose{Kind: ProseBriefing, Frame: g.Briefing}
}
