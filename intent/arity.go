package intent

import (
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
)

// requirement — какие аргументы глагол обязан получить. Схема этого выразить
// не может: «target обязателен, если verb=talk_to» — условие между полями, а
// не форма поля. Поэтому арность проверяется кодом и объявляется в промпте.
type requirement struct {
	Target  bool
	Topic   bool
	Node    bool
	Item    bool
	Ability bool
	Text    bool
	Facts   int
}

// arity повторяет грамматику структурированного парсера CLI. Один источник
// истины на два входа: расхождение здесь означало бы, что свободный текст и
// команда ведут себя по-разному.
var arity = map[core.Verb]requirement{
	"look":            {},
	"question":        {Target: true, Topic: true},
	"ask_about":       {Target: true, Topic: true},
	"cross_reference": {Target: true, Topic: true},
	"move_zone":       {Node: true},
	"use_item":        {Item: true},
	// present — предъявить предмет. Предмет обязателен, цель нет: форма
	// «предъявить узлу» дизайном оставлена на будущее, и грамматика её не
	// запрещает.
	"present":     {Item: true},
	"use_ability": {Ability: true},
	"theorize":    {Text: true},
	"say":         {Text: true},
	"emote":       {Text: true},
	"compare":     {Facts: 2},
}

// requires возвращает требования глагола. По умолчанию глагол берёт одну
// цель — так же, как в CLI.
func requires(v core.Verb) requirement {
	if r, ok := arity[v]; ok {
		return r
	}
	return requirement{Target: true}
}

// missing сообщает, какого аргумента не хватает, и что спросить у игрока.
// Пустая строка означает, что всё на месте.
func (r requirement) missing(raw reply) string {
	switch {
	case r.Target && raw.Target == "":
		return "к кому или к чему? назови, кто из присутствующих"
	case r.Topic && raw.Topic == "":
		return "о чём именно спросить?"
	case r.Node && raw.Node == "":
		return "куда идти?"
	case r.Item && raw.Item == "":
		return "чем именно?"
	case r.Ability && raw.Ability == "":
		return "какую способность применить?"
	case r.Text && strings.TrimSpace(raw.Text) == "":
		return "что именно ты хочешь сказать?"
	case r.Facts > 0 && len(raw.Facts) < r.Facts:
		return "какие два известных факта сопоставить?"
	}
	return ""
}

// arityBrief — требования в виде строки для промпта. Порядок стабилен:
// промпт входит в кэшируемый префикс.
func arityBrief() string {
	groups := map[string][]string{}
	for _, d := range core.AllVerbs() {
		var need []string
		r := requires(d.Verb)
		if r.Target {
			need = append(need, "target")
		}
		if r.Topic {
			need = append(need, "topic")
		}
		if r.Node {
			need = append(need, "node")
		}
		if r.Item {
			need = append(need, "item")
		}
		if r.Ability {
			need = append(need, "ability")
		}
		if r.Text {
			need = append(need, "text")
		}
		if r.Facts > 0 {
			need = append(need, "два facts")
		}
		key := "ничего"
		if len(need) > 0 {
			key = strings.Join(need, " и ")
		}
		groups[key] = append(groups[key], string(d.Verb))
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		sort.Strings(groups[k])
		b.WriteString("  " + strings.Join(groups[k], ", ") + " — " + k + "\n")
	}
	return b.String()
}
