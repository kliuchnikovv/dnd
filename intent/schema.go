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
func verbNames() []string {
	out := make([]string, 0, len(core.Verbs))
	for _, d := range core.AllVerbs() {
		out = append(out, string(d.Verb))
	}
	sort.Strings(out)
	return out
}

// Schema — JSON-схема ответа парсера, выведенная из реестра глаголов.
// additionalProperties:false обязателен: без него strict-режим не даёт
// гарантии, а лишние поля молча приезжают в канон.
func Schema() map[string]any {
	str := map[string]any{"type": "string"}
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
				"type": "string", "enum": verbNames(),
				"description": "глагол из реестра; обязателен при outcome=intent",
			},
			"target":  map[string]any{"type": "string", "description": "id сущности из списка присутствующих"},
			"topic":   map[string]any{"type": "string", "description": "id факта из банка тем парти"},
			"node":    map[string]any{"type": "string", "description": "id смежного открытого узла"},
			"facts":   map[string]any{"type": "array", "items": str, "description": "id фактов для compare"},
			"item":    str,
			"ability": str,
			"text":    map[string]any{"type": "string", "description": "свободный текст для theorize, say, emote"},
			"clarify": map[string]any{"type": "string", "description": "вопрос игроку в характере при outcome=clarify"},
			"reason":  map[string]any{"type": "string", "description": "чего не хватило словарю при outcome=unsupported"},
		},
	}
}

// SchemaJSON — схема в том виде, в котором уходит провайдеру.
func SchemaJSON() string {
	b, err := json.Marshal(Schema())
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
