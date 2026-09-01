package cli

import (
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/store"
)

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
	id := speakerID(g, in, t)
	if id == "" {
		return Prose{}, false
	}
	return Prose{
		Kind:     ProseReply,
		Frame:    g.Flavour(t.FlavourKey),
		Scene:    sceneOf(g),
		Outcome:  outcomeOf(g, t),
		Speaking: g.DB.Entities[id].Name,
		Speaker:  speakerCard(g, id),
		State:    core.StateDigest(g, t).Lines(),
	}, true
}

// speakerCard — карточка NPC для KindReply. Живёт здесь, а не в core, потому
// что core про домен, а карточка — про то, что Мастер прочтёт в промпте;
// склеиваем её из уже существующих геттеров Dossiers.
//
// Правды дела в карточку не кладём: KnowsAbout/TalksAbout остаются в ядре и
// вплывают в реплику только через авторскую рамку. Здесь только человек —
// голос, быт, отношение, незакрытое, желания, память разговора.
func speakerCard(g *core.Game, id store.EntityID) *master.Speaker {
	if g == nil || g.D == nil || id == "" {
		return nil
	}
	e := g.DB.Entities[id]
	return &master.Speaker{
		Name:        e.Name,
		Kind:        entityKindWord(g, id),
		Voice:       g.D.Voice(id),
		Life:        g.D.Life(id),
		Disposition: g.D.Disposition(id),
		OpenThreads: g.D.OpenThreads(id),
		Wants:       g.D.Wants(id),
		TalksAbout:  g.D.TalksAbout(id),
		Recent:      g.D.Recent(id),
		Summary:     g.D.View(id).Summary,
	}
}

// entityKindWord — роль NPC для карточки. Берётся из авторского Voice, если
// автор её туда положил (первая строка «стражник у ворот» — этого достаточно
// для промпта); иначе пусто — врать про роль хуже, чем промолчать.
func entityKindWord(g *core.Game, id store.EntityID) string {
	// Явного поля роли у Entity нет: kind — это только npc/location/character.
	// Оставляем пусто; при необходимости автор дела кладёт роль в Voice первой
	// фразой, и Мастер её читает целиком.
	_ = g
	_ = id
	return ""
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

// clarifyFallback — отклик на неясное обращение, когда Мастера нет: игра без
// моделей работает как раньше, а молчание игрок читает как поломку.
const clarifyFallback = "Ваши слова повисли без ответа — непонятно, к кому вы обращаетесь."

// ClarifyProse — диегетический отклик на неясное обращение: мир не понял игрока
// и ждёт, пока он скажет яснее. Рамка кодовая (авторской под это нет), гвардить
// нечем — состояние не менялось.
func ClarifyProse(g *core.Game) Prose {
	return Prose{
		Kind:  ProseClarify,
		Frame: clarifyFallback,
		Scene: sceneOf(g),
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
