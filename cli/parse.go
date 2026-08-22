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
	CmdItems
	CmdHelp
	CmdQuit
	CmdRest
	CmdSurvey
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
	// push — приставка, а не команда: решение о риске принимается про
	// конкретный бросок. Само по себе слово ничего не значит.
	push := false
	if fields[0] == "push" {
		if len(fields) == 1 {
			return Command{}, errors.New("push требует действия: push examine e_body")
		}
		push, fields = true, fields[1:]
	}
	head, rest := fields[0], fields[1:]

	switch head {
	case "survey":
		return Command{Kind: CmdSurvey}, nil
	case "facts":
		return Command{Kind: CmdFacts}, nil
	case "state":
		return Command{Kind: CmdState}, nil
	case "clocks":
		return Command{Kind: CmdClocks}, nil
	case "items":
		return Command{Kind: CmdItems}, nil
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

	cmd := Command{Kind: CmdAction, Intent: core.Intent{Verb: def.Verb, Push: push}}
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
	case "question", "ask_about":
		// Тема необязательна: идентификатор неизвестного факта игроку негде
		// взять, и без открытого вопроса люди для него немы. Названная тема —
		// это уточнение («спроси именно об этом»), а не условие разговора.
		if len(rest) == 0 || len(rest) > 2 {
			return Command{}, errors.New(head + " требует источник и, если надо, тему")
		}
		cmd.Intent.Args.Target = entityID(rest[0])
		if len(rest) == 2 {
			cmd.Intent.Args.Topic = factID(rest[1])
		}
		return cmd, nil
	case "cross_reference":
		// Сверка без факта бессмысленна: сверяют запись С ЧЕМ-ТО известным.
		if len(rest) != 2 {
			return Command{}, errors.New("cross_reference требует запись и известный факт")
		}
		cmd.Intent.Args.Target = entityID(rest[0])
		cmd.Intent.Args.Topic = factID(rest[1])
		return cmd, nil
	case "present":
		// Адресат необязателен: форма «предъявить узлу» дизайном оставлена на
		// будущее, и грамматика её не запрещает.
		if len(rest) == 0 || len(rest) > 2 {
			return Command{}, errors.New("present требует предмет и, если надо, адресата")
		}
		cmd.Intent.Args.Item = string(itemID(rest[0]))
		if len(rest) == 2 {
			cmd.Intent.Args.Target = entityID(rest[1])
		}
		return cmd, nil
	case "use_item":
		if len(rest) != 1 {
			return Command{}, errors.New("use_item требует предмет")
		}
		// Объект этого глагола живёт в двух таблицах: инструмент бывает пропом
		// узла и носимым предметом. Голое имя — проп (частый случай),
		// носимое называется явно, с префиксом i_.
		cmd.Intent.Args.Item = string(propID(rest[0]))
		return cmd, nil
	case "use_ability":
		if len(rest) != 1 {
			return Command{}, errors.New("use_ability требует способность")
		}
		cmd.Intent.Args.Ability = rest[0]
		return cmd, nil
	case "look":
		// Цель необязательна: «look» осматривается вокруг, «look p_crates»
		// разглядывает деталь.
		if len(rest) == 1 {
			cmd.Intent.Args.Target = entityID(rest[0])
		}
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
func itemID(s string) store.ItemID     { return store.ItemID(withPrefix(s, "i_")) }
func propID(s string) store.PropID     { return store.PropID(withPrefix(s, "p_")) }

// knownPrefixes — идентификаторы, которые уже размечены. Дописывать «e_»
// пропу нельзя: игрок ушёл бы в несуществующую сущность, и отказ сообщил бы
// ему, что цель была инертной.
var knownPrefixes = []string{"e_", "p_", "f_", "n_", "i_"}

func withPrefix(s, prefix string) string {
	for _, p := range knownPrefixes {
		if strings.HasPrefix(s, p) {
			return s
		}
	}
	return prefix + s
}
