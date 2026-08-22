package cli

import (
	"regexp"
	"strings"
	"testing"
)

// Справка — единственный учитель игрока: других инструкций у него нет. Строка
// справки, чья форма не разбирается парсером, не просто бесполезна — она
// уводит в тупик и выглядит как поломка игры.
//
// Плейтест это и показал: игрок трижды пробовал документированные формы
// `search <локация>` и `stake_out <локация>`, получал «такой сущности в деле
// нет» и решил, что механика ему недоступна.
//
// Тест держит справку контрактом: у каждого заполнителя есть образец значения,
// и форма из справки обязана разобраться в интент с ЭТИМ значением в ЭТОМ поле.
var placeholders = map[string]struct {
	sample string
	field  func(Command) string
}{
	"<цель>":        {"e_toke", func(c Command) string { return string(c.Intent.Args.Target) }},
	"<вещь>":        {"i_writ", func(c Command) string { return c.Intent.Args.Item }},
	"<источник>":    {"e_toke", func(c Command) string { return string(c.Intent.Args.Target) }},
	"<предмет>":     {"e_body", func(c Command) string { return string(c.Intent.Args.Target) }},
	"<запись>":      {"e_ledger", func(c Command) string { return string(c.Intent.Args.Target) }},
	"<тема>":        {"f_x", func(c Command) string { return string(c.Intent.Args.Topic) }},
	"<факт>":        {"f_x", func(c Command) string { return string(c.Intent.Args.Topic) }},
	"<узел>":        {"n_quay", func(c Command) string { return string(c.Intent.Args.Node) }},
	"<текст>":       {"догадка", func(c Command) string { return c.Intent.Args.Text }},
	"<способность>": {"a_x", func(c Command) string { return c.Intent.Args.Ability }},
	"[кому]":        {"e_toke", func(c Command) string { return string(c.Intent.Args.Target) }},
}

// exempt — формы, которые проверять этим тестом нечем. Каждая с причиной:
// push это приставка к другому действию, а не команда со своим аргументом.
var exempt = map[string]bool{"push": true}

// factPairs — глаголы, чьи «факты» ложатся не в тему, а в пару фактов.
var factPairs = map[string]bool{"compare": true}

var helpLine = regexp.MustCompile(`^([a-z_]+)((?:\s+[<\[][^>\]]+[>\]])*)`)

func TestHelpFormsActuallyParse(t *testing.T) {
	for _, line := range strings.Split(Render{}.Help(), "\n") {
		m := helpLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue // «реплика», rest short|long и прочие особые формы
		}
		verb, args := m[1], strings.Fields(m[2])
		if len(args) == 0 || exempt[verb] {
			continue // команды без аргументов и приставки проверять нечем
		}
		if factPairs[verb] {
			// Пара фактов ложится в Facts, а не в тему: проверяем состав пары.
			cmd, err := Parse(verb + " f_a f_b")
			if err != nil {
				t.Errorf("справка обещает «%s %s», а парсер это не берёт: %v",
					verb, strings.Join(args, " "), err)
				continue
			}
			if len(cmd.Facts) != 2 || cmd.Facts[0] != "f_a" || cmd.Facts[1] != "f_b" {
				t.Errorf("справка обещает «%s %s», а факты разобрались как %v",
					verb, strings.Join(args, " "), cmd.Facts)
			}
			continue
		}

		var parts []string
		var checks []func(Command) string
		var names []string
		for _, a := range args {
			ph, ok := placeholders[a]
			if !ok {
				t.Errorf("%s: заполнитель %s не описан в тесте — добавь образец", verb, a)
				continue
			}
			parts = append(parts, ph.sample)
			checks = append(checks, ph.field)
			names = append(names, a)
		}
		if len(parts) != len(args) {
			continue
		}

		cmd, err := Parse(verb + " " + strings.Join(parts, " "))
		if err != nil {
			t.Errorf("справка обещает «%s %s», а парсер это не берёт: %v",
				verb, strings.Join(args, " "), err)
			continue
		}
		for i, check := range checks {
			if got := check(cmd); got != parts[i] {
				t.Errorf("справка обещает «%s %s»: %s должно попасть в своё поле, "+
					"а поле содержит %q", verb, strings.Join(args, " "), names[i], got)
			}
		}
	}
}
