// Package cli — структурированный ввод и рендер. Никакого естественного языка:
// «question ivar ledger», а не «спрошу кузнеца про книгу».
package cli

import (
	"errors"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

type CommandKind int

const (
	CmdNone CommandKind = iota
	CmdAction
	CmdCompare
	CmdAccuse
	CmdFacts
	CmdState
	CmdClocks
	CmdHelp
	CmdQuit
	CmdRest
)

type Command struct {
	Kind   CommandKind
	Intent core.Intent
	Facts  []store.FactID
	// Text — остаток строки вне прямой речи либо аргумент бесструктурных
	// команд. Для речи в нём лежит указание адресата.
	Text string
}

var ErrUnknownVerb = errors.New("неизвестное действие")

// speechPairs — парные обрамления прямой речи. Игрок сам помечает, что
// отыгрывает персонажа, и на разбор такой строки не тратится вызов модели.
var speechPairs = [][2]string{{`"`, `"`}, {"«", "»"}, {"'", "'"}, {"“", "”"}}

// speechDashes — речь через тире: в этой форме репликой считается вся строка.
var speechDashes = []string{"—", "–", "-"}

// Speech выделяет прямую речь и остаток строки.
//
// Кавычки ищутся ГДЕ УГОДНО, а не только в начале: «Обращаясь к Нильсу —
// "а ты ничего не видел?"» это обычная запись, и остаток нужен, чтобы понять,
// к кому обращаются.
func Speech(line string) (said, rest string, ok bool) {
	line = strings.TrimSpace(line)
	for _, pair := range speechPairs {
		open := strings.Index(line, pair[0])
		if open < 0 {
			continue
		}
		tail := line[open+len(pair[0]):]
		close := strings.Index(tail, pair[1])
		if close < 0 {
			continue
		}
		said = strings.TrimSpace(tail[:close])
		if said == "" {
			continue
		}
		rest = strings.Join(strings.Fields(line[:open]+" "+tail[close+len(pair[1]):]), " ")
		return said, rest, true
	}
	for _, dash := range speechDashes {
		if !strings.HasPrefix(line, dash) {
			continue
		}
		if said = strings.TrimSpace(strings.TrimPrefix(line, dash)); said != "" {
			return said, "", true
		}
	}
	return "", "", false
}

// Parse разбирает одну строку ввода. Пустые строки и строки-комментарии дают
// CmdNone: скриптовый прогон читает те же файлы, что пишет человек.
func Parse(line string) (Command, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Command{Kind: CmdNone}, nil
	}
	// Прямая речь опознаётся до всего остального: «Что нового?» в кавычках —
	// это реплика персонажа, а не команда осмотреться.
	if said, rest, ok := Speech(line); ok {
		return Command{Kind: CmdAction, Text: rest, Intent: core.Intent{
			Verb: "say", Args: core.Args{Text: said},
		}}, nil
	}
	fields := strings.Fields(line)
	head, rest := fields[0], fields[1:]

	switch head {
	case "facts":
		return Command{Kind: CmdFacts}, nil
	case "state":
		return Command{Kind: CmdState}, nil
	case "clocks":
		return Command{Kind: CmdClocks}, nil
	case "help":
		return Command{Kind: CmdHelp}, nil
	case "accuse":
		return Command{Kind: CmdAccuse}, nil
	case "quit", "exit":
		return Command{Kind: CmdQuit}, nil
	case "rest":
		if len(rest) != 1 || (rest[0] != "short" && rest[0] != "long") {
			return Command{}, errors.New("rest требует short или long")
		}
		return Command{Kind: CmdRest, Text: rest[0]}, nil
	case "compare":
		if len(rest) != 2 {
			return Command{}, errors.New("compare требует ровно два факта")
		}
		return Command{Kind: CmdCompare, Facts: []store.FactID{
			factID(rest[0]), factID(rest[1]),
		}}, nil
	}

	def, ok := core.LookupVerb(head)
	if !ok {
		return Command{}, ErrUnknownVerb
	}

	cmd := Command{Kind: CmdAction, Intent: core.Intent{Verb: def.Verb}}
	switch head {
	case "theorize", "say", "emote":
		cmd.Intent.Args.Text = strings.Join(rest, " ")
		return cmd, nil
	case "move_zone":
		if len(rest) != 1 {
			return Command{}, errors.New("move_zone требует узел")
		}
		cmd.Intent.Args.Node = nodeID(rest[0])
		return cmd, nil
	case "question", "ask_about", "cross_reference":
		if len(rest) != 2 {
			return Command{}, errors.New(head + " требует источник и тему")
		}
		cmd.Intent.Args.Target = entityID(rest[0])
		cmd.Intent.Args.Topic = factID(rest[1])
		return cmd, nil
	case "use_item":
		if len(rest) != 1 {
			return Command{}, errors.New("use_item требует предмет")
		}
		cmd.Intent.Args.Item = rest[0]
		return cmd, nil
	case "use_ability":
		if len(rest) != 1 {
			return Command{}, errors.New("use_ability требует способность")
		}
		cmd.Intent.Args.Ability = rest[0]
		return cmd, nil
	case "look":
		return cmd, nil
	}

	if len(rest) != 1 {
		return Command{}, errors.New(head + " требует одну цель")
	}
	cmd.Intent.Args.Target = entityID(rest[0])
	return cmd, nil
}

// Префиксы можно не писать: «ivar» и «e_ivar» — одно и то же. Идентификаторы
// из вывода facts вставляются как есть, короткие имена набираются быстрее.
func entityID(s string) store.EntityID { return store.EntityID(withPrefix(s, "e_")) }
func factID(s string) store.FactID     { return store.FactID(withPrefix(s, "f_")) }
func nodeID(s string) store.NodeID     { return store.NodeID(withPrefix(s, "n_")) }

func withPrefix(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}
