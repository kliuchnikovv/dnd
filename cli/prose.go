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

// ReplyProse — прямая речь NPC в ответ на ход игрока. Второй результат ложен,
// когда отвечать некому (не разговорный ход) или нет авторской рамки исхода, на
// которой реплику можно заземлить. Заземляется той же рамкой, что и исход:
// слова персонажа не должны расходиться с авторским беатом.
func ReplyProse(g *core.Game, in core.Intent, t core.TurnResult) (Prose, bool) {
	if t.Refused || t.FlavourKey == "" {
		return Prose{}, false
	}
	who := speakerName(g, in, t)
	if who == "" {
		return Prose{}, false
	}
	return Prose{
		Kind:     ProseReply,
		Frame:    g.Flavour(t.FlavourKey),
		Scene:    sceneOf(g),
		Outcome:  outcomeOf(g, t),
		Speaking: who,
		State:    core.StateDigest(g, t).Lines(),
	}, true
}

// ProbeProse — отклик мира на свободную пробу (ввод, который словарь не выразил
// и который не задел авторскую цель). Ход проба не тратит, канон не меняет —
// чистое повествование. Текст пробы едет Мастеру слотом outcome «Игрок пробует:
// …» (так же, как cmd/dnd), рамка кодовая (авторской под пробу нет).
func ProbeProse(g *core.Game, probe core.Probe) Prose {
	return Prose{
		Kind:    ProseProbe,
		Frame:   probeFallback,
		Probe:   probe.Text,
		Outcome: []string{"Игрок пробует: " + probe.Text},
		Scene:   sceneOf(g),
		State:   core.StateDigest(g, core.TurnResult{}).Lines(),
	}
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
