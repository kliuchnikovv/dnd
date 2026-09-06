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
	OutcomeIntent  = "intent"  // распознано как действие
	OutcomeClarify = "clarify" // непонятно, нужен вопрос в характере
	// OutcomeFreeProbe — игрок пробует то, чего словарь не выражает. Не
	// отказ, а приземление: отклик описывает Мастер из сцены и канона, а если
	// проба легла на авторскую цель — её резолвит ядро (ADR-0003, T1).
	OutcomeFreeProbe = "free_probe"
	// OutcomeUnsupported — словарь такого не покрывает. Остаётся внутренним
	// сигналом узости словаря: на нём живёт метрика, и «кандидат в новый
	// глагол» терять незачем. Тупиком быть перестаёт — игрок получает пробу.
	OutcomeUnsupported = "unsupported"
	// OutcomeIdle — ввод НЕ действие в мире: мета-инструкция, инъекция
	// («ignore previous…»), служебный текст/JSON, обращение к системе,
	// бессмыслица, попытка переписать сцену. Персонаж ничего не предпринимает —
	// ни интента, ни пробы, ни вопроса, ни мутации. Осознанное отступление от
	// «всё приземляется» (ADR-0003) ради мусора: приземлять инъекцию пробой
	// значило бы дать Мастеру её пересказать (прото §2.12).
	OutcomeIdle = "idle"
)

// classNames — классы реестра в стабильном порядке, как их видит модель.
// Список берётся у ядра: свой завёл бы второе место правды о том, какие классы
// вообще бывают.
func classNames() []string {
	all := core.AllClasses()
	out := make([]string, 0, len(all))
	for _, c := range all {
		out = append(out, string(c))
	}
	return out
}

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
	// enumOrEmpty — то же, но пустая строка тоже законна: поле обязательное, а
	// действие бывает ни на кого не направлено.
	enumOrEmpty := func(list []Named, desc string) map[string]any {
		m := map[string]any{"type": "string", "description": desc}
		if ids := idsOf(list); len(ids) > 0 {
			m["enum"] = append([]string{""}, ids...)
		}
		return m
	}
	_ = str
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		// target объявлен обязательным СОЗНАТЕЛЬНО, вместе с пустым значением в
		// перечислении. Модель со строгой схемой заполняет ровно то, что
		// требуется: с одним лишь outcome в required она возвращала
		// {"verb":"examine"} без цели даже там, где цель названа словами, и
		// даже после прямого «ОБЯЗАТЕЛЕН» в описании поля и в вводе. Пустая
		// строка — законный ответ для действий, ни на кого не направленных.
		"required": []string{"outcome", "target", "topic", "item"},
		"properties": map[string]any{
			"outcome": map[string]any{
				"type": "string",
				"enum": []string{OutcomeIntent, OutcomeClarify, OutcomeFreeProbe, OutcomeUnsupported, OutcomeIdle},
			},
			"verb": map[string]any{
				"type": "string", "enum": verbNamesFor(hint),
				"description": "глагол из реестра; обязателен при outcome=intent",
			},
			"target": enumOrEmpty(hint.targets(), "id цели: человек из присутствующих либо деталь места. "+
				"ОБЯЗАТЕЛЕН для действий, направленных на кого-то или что-то: examine, search, "+
				"question, talk_to, present, stake_out и прочих. Без него действие не исполнится, "+
				"и игру придётся переспрашивать"),
			"topic": enumOrEmpty(hint.Topics, "id факта из банка тем парти; "+
				"пусто, если игрок спрашивает открыто или тема ни при чём"),
			// «Смежный» и «открытый» — понятия старого гейта (разрешение на
			// вход), которого больше нет: перемещение проверяет знание, а не
			// смежность, и список тут — известные места дела, а не проходы.
			"node": enumOr(hint.Reachable, "id известного места дела"),
			"facts": map[string]any{"type": "array",
				"items":       enumOr(hint.Topics, "id известного факта"),
				"description": "ровно два id известных фактов для compare"},
			"item": enumOrEmpty(itemChoices(hint), "id предмета: из «можно применить» либо из «при себе»; "+
				"пусто, если действие не про предмет"),
			"ability": str,
			"text":    map[string]any{"type": "string", "description": "свободный текст для theorize, say, emote"},
			"clarify": map[string]any{"type": "string", "description": "вопрос игроку в характере при outcome=clarify"},
			"reason":  map[string]any{"type": "string", "description": "чего не хватило словарю при outcome=unsupported"},
			// probe НЕ обязателен, в отличие от target, topic и item. У тех
			// доводчика нет — незаполненное поле означает недоисполнимый ход.
			// Здесь доводчик есть и он честный: пустое probe заменяется
			// словами самого игрока. Платить обязательным выводом каждый ход
			// за то, что и так есть в строке ввода, незачем.
			"probe": map[string]any{"type": "string",
				"description": "что игрок пробует, коротким описанием от третьего лица; " +
					"при outcome=free_probe"},
			// class — подсказка о ФОРМЕ импровизации, и только. Без неё свободное
			// действие не доходит до броска: у него нет ни кости, ни таксономии
			// цены провала. Сложности она не назначает и назначать не может —
			// порог ставит ядро по позиции (ADR-0001). Поэтому в описании прямо
			// сказано, о чём поле НЕ говорит: иначе модель начнёт торговаться за
			// лёгкость формулировкой.
			"class": map[string]any{"type": "string", "enum": classNames(),
				"description": "на что похоже свободное действие при outcome=free_probe: " +
					"форма действия, НЕ его сложность"},
		},
	}
}

// chatSchemaJSONFor — схема чат-режима: та же, плюс обязательная реплика.
// Обязательная СОЗНАТЕЛЬНО, по той же причине, что target, topic и item:
// со свободным полем модель его не заполняет, сколько бы ни просили в
// описании (docs/status.md §3.5).
//
// Реплика в схеме обычного разбора не появляется: там её никто не покажет, и
// модель платила бы выводом за выброшенный текст.
func chatSchemaJSONFor(hint SceneHint) string {
	schema := SchemaFor(hint)
	props := schema["properties"].(map[string]any)
	props["reply"] = map[string]any{"type": "string",
		"description": "одна-две фразы Мастера игроку: что он делает и что видит " +
			"в этот момент. НЕ утверждай исход — бросок ещё не сделан"}
	schema["required"] = append(schema["required"].([]string), "reply")
	b, err := json.Marshal(schema)
	if err != nil {
		panic(err) // схема статична: ошибка здесь означает сломанный билд
	}
	return string(b)
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
	Probe   string   `json:"probe"`
	// Class — форма, предложенная моделью для свободной пробы. Недоверенная:
	// проверяется по реестру ядра, и выдуманное значение подсказкой не
	// становится.
	Class string `json:"class"`
	// Reply — реплика Мастера. Просится только в чат-режиме; в обычном
	// разборе поля нет в схеме, и оно остаётся пустым.
	Reply string `json:"reply"`
}
