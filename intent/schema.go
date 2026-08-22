// Package intent переводит свободный текст игрока в core.Intent через модель.
//
// Пакет лежит НАД доменом: он импортирует core и llm, а они его не знают.
// Гарантия честности двухслойная. Схема, переданная модели, задаёт форму
// ответа; проверка в коде задаёт его смысл: глагол обязан существовать в
// реестре, а цель, тема и узел — присутствовать в сцене. Схема без проверки
// значений пропустила бы ссылку на сущность, которой нет.
package intent

import (
	"encoding/json"
	"sort"

	"github.com/kliuchnikovv/dnd/core"
)

// Outcome — что модель сделала со вводом.
const (
	OutcomeIntent      = "intent"      // распознано как действие
	OutcomeClarify     = "clarify"     // непонятно, нужен вопрос в характере
	OutcomeUnsupported = "unsupported" // словарь такого не покрывает
)

// verbNames возвращает глаголы реестра в стабильном порядке. Схема обязана
// быть побайтово одинаковой между вызовами: у провайдеров грамматики
// кэшируются, и плавающая схема этот кэш обнуляет.
func verbNames() []string { return verbNamesFor(SceneHint{}) }

// verbNamesFor — глаголы, которые в этой сцене вообще исполнимы. Глагол,
// чей обязательный аргумент взять негде, из грамматики уходит: иначе модель
// его выбирает, аргумент заполнить не может, и игра спрашивает игрока о том,
// на что нет ответа.
func verbNamesFor(hint SceneHint) []string {
	out := make([]string, 0, len(core.Verbs))
	for _, d := range core.AllVerbs() {
		// Предметные глаголы берут предметы из РАЗНЫХ мест: use_item — из
		// пропов узла с меткой tool, present — из носимого. Общий список
		// вернул бы use_item в грамматику из-за бумаги в кармане, и модель
		// уехала бы в бросок по пропу, которого в узле нет.
		if src := itemSource(d.Verb, hint); requires(d.Verb).Item && len(src) == 0 {
			continue
		}
		out = append(out, string(d.Verb))
	}
	sort.Strings(out)
	return out
}

// itemSource — откуда этот глагол берёт предмет.
func itemSource(v core.Verb, hint SceneHint) []Named {
	if v == "present" {
		return hint.Carried
	}
	return hint.Tools
}

// itemChoices — всё, что вообще можно назвать в поле item: поле одно на два
// глагола, и перечисление обязано покрывать оба, иначе предъявить носимое
// нельзя.
func itemChoices(hint SceneHint) []Named {
	out := make([]Named, 0, len(hint.Tools)+len(hint.Carried))
	out = append(out, hint.Tools...)
	return append(out, hint.Carried...)
}

// Schema — схема без привязки к сцене. Нужна для тестов и документации;
// парсер использует SchemaFor, потому что перечисления из сцены работают
// сильнее любой инструкции в промпте.
func Schema() map[string]any { return SchemaFor(SceneHint{}) }

// SchemaFor строит схему под конкретную сцену: target, topic и node получают
// enum из фактически присутствующих идентификаторов. Модель не может выдумать
// сущность не потому, что её попросили, а потому, что грамматика не даёт.
//
// Цена: схема меняется вместе со сценой, поэтому кэш грамматики у провайдера
// живёт на узел, а не на всё дело. Это дешевле, чем разбирать ссылки на
// несуществующих людей.
func SchemaFor(hint SceneHint) map[string]any {
	str := map[string]any{"type": "string"}
	enumOr := func(list []Named, desc string) map[string]any {
		m := map[string]any{"type": "string", "description": desc}
		if ids := idsOf(list); len(ids) > 0 {
			m["enum"] = ids
		}
		return m
	}
	_ = str
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"outcome"},
		"properties": map[string]any{
			"outcome": map[string]any{
				"type": "string",
				"enum": []string{OutcomeIntent, OutcomeClarify, OutcomeUnsupported},
			},
			"verb": map[string]any{
				"type": "string", "enum": verbNamesFor(hint),
				"description": "глагол из реестра; обязателен при outcome=intent",
			},
			"target": enumOr(hint.Entities, "id сущности из списка присутствующих"),
			"topic":  enumOr(hint.Topics, "id факта из банка тем парти"),
			"node":   enumOr(hint.Reachable, "id смежного открытого узла"),
			"facts": map[string]any{"type": "array",
				"items":       enumOr(hint.Topics, "id известного факта"),
				"description": "ровно два id известных фактов для compare"},
			"item":    enumOr(itemChoices(hint), "id предмета: из «можно применить» либо из «при себе»"),
			"ability": str,
			"text":    map[string]any{"type": "string", "description": "свободный текст для theorize, say, emote"},
			"clarify": map[string]any{"type": "string", "description": "вопрос игроку в характере при outcome=clarify"},
			"reason":  map[string]any{"type": "string", "description": "чего не хватило словарю при outcome=unsupported"},
		},
	}
}

func idsOf(list []Named) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, n.ID)
	}
	return out
}

// SchemaJSON — схема сцены в том виде, в котором уходит провайдеру.
func SchemaJSON() string { return schemaJSONFor(SceneHint{}) }

func schemaJSONFor(hint SceneHint) string {
	b, err := json.Marshal(SchemaFor(hint))
	if err != nil {
		panic(err) // схема статична: ошибка здесь означает сломанный билд
	}
	return string(b)
}

// reply — форма ответа модели. Поля читаются только после проверки значений.
type reply struct {
	Outcome string   `json:"outcome"`
	Verb    string   `json:"verb"`
	Target  string   `json:"target"`
	Topic   string   `json:"topic"`
	Node    string   `json:"node"`
	Facts   []string `json:"facts"`
	Item    string   `json:"item"`
	Ability string   `json:"ability"`
	Text    string   `json:"text"`
	Clarify string   `json:"clarify"`
	Reason  string   `json:"reason"`
}
