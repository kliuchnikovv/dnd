package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/naming"
	"github.com/kliuchnikovv/dnd/store"
)

// Session гоняет один и тот же цикл и для интерактивного REPL, и для скрипта:
// прогон воспроизводится парой (seed, файл команд).
type Session struct {
	Game   *core.Game
	In     io.Reader
	Out    io.Writer
	r      Render
	sc     *bufio.Scanner
	interp Interpreter
	voicer Voicer
	turn   int
}

func NewSession(g *core.Game, in io.Reader, out io.Writer) *Session {
	return &Session{Game: g, In: in, Out: out}
}

func (s *Session) Run() error {
	fmt.Fprint(s.Out, s.r.Scene(s.Game))
	s.sc = bufio.NewScanner(s.In)
	for s.sc.Scan() {
		line := s.sc.Text()
		s.turn++
		cmd, err := Parse(line)
		if err != nil {
			s.interpret(line, err)
			continue
		}
		if done := s.dispatch(cmd); done {
			return nil
		}
	}
	return s.sc.Err()
}

func (s *Session) dispatch(cmd Command) bool {
	g, r := s.Game, s.r
	switch cmd.Kind {
	case CmdNone:
	case CmdQuit:
		return true
	case CmdHelp:
		fmt.Fprint(s.Out, r.Help())
	case CmdFacts:
		fmt.Fprint(s.Out, r.Facts(g))
	case CmdState:
		fmt.Fprint(s.Out, r.State(g))
	case CmdClocks:
		fmt.Fprint(s.Out, r.Clocks(g))
	case CmdCompare:
		fmt.Fprint(s.Out, r.Turn(g, g.Compare(cmd.Facts[0], cmd.Facts[1])))
	case CmdAccuse:
		s.accuse()
	case CmdRest:
		kind := core.RestShort
		if cmd.Text == "long" {
			kind = core.RestLong
		}
		if clocks := g.RestPreview(kind); len(clocks) > 0 {
			fmt.Fprintf(s.Out, "длинный отдых продвинет часы: %v\n", clocks)
		}
		fmt.Fprint(s.Out, r.Turn(g, g.Rest(kind)))
	case CmdAction:
		cmd.Intent.Actor = g.Actor
		s.addressee(&cmd.Intent)
		res := g.Apply(cmd.Intent)
		fmt.Fprint(s.Out, r.Turn(g, res))
		s.afterAction(cmd.Intent, res)
	}
	return false
}

// accuse собирает форму из доступных токенов. Ошибочная форма не сообщает,
// какой слот неверен: иначе слоты брутфорсятся по одному.
func (s *Session) accuse() {
	g := s.Game
	form := accusation.Form{}
	slots := []struct {
		name string
		dst  *store.Token
	}{
		{"who", &form.Who}, {"how", &form.How},
		{"when", &form.When}, {"why", &form.Why},
	}
	for _, slot := range slots {
		avail := g.AvailableTokens(slot.name)
		if len(avail) == 0 {
			fmt.Fprintf(s.Out, "слот %s пуст: нужных фактов ещё нет\n", slot.name)
			return
		}
		parts := make([]string, len(avail))
		for i, a := range avail {
			parts[i] = string(a)
		}
		fmt.Fprintf(s.Out, "%s: %s\n> ", slot.name, strings.Join(parts, " | "))
		if !s.sc.Scan() {
			return
		}
		*slot.dst = store.Token(strings.TrimSpace(s.sc.Text()))
	}
	res := g.Accuse(form)
	switch {
	case res.Refused:
		fmt.Fprintf(s.Out, "нельзя: %s\n", res.Refusal)
	case res.Correct:
		fmt.Fprintf(s.Out, "Обвинение верно. Попыток: %d.\n", res.Attempt)
	default:
		fmt.Fprintf(s.Out, "В обвинении есть ошибки. Попыток: %d.\n", res.Attempt)
	}
	for _, c := range res.Fired {
		fmt.Fprintf(s.Out, "  ⏱ %s\n", g.Flavour(c.FlavourKey))
	}
}

// afterAction — последствия действия, не выражаемые мутацией. Перемещение
// живёт здесь, а не в ядре, и это известный шов: ядро резолвит бросок,
// презентация меняет узел. Вынесено, чтобы структурированный ввод и
// переводчик свободного текста вели себя одинаково.
//
// ЧАСТИЧНО по таксономии move — «попал + ухудшение позиции»: игрок доходит
// и платит. Требовать УСПЕХ значило бы брать цену прихода, не давая прийти.
func (s *Session) afterAction(in core.Intent, res core.TurnResult) {
	s.speak(in, res)
	if in.Verb == "move_zone" && res.Res != nil && res.Res.Class >= core.OutcomePartial {
		s.Game.Node = in.Args.Node
		fmt.Fprint(s.Out, s.r.Scene(s.Game))
	}
}

// addressee подставляет собеседника прямой речи. Игрок, сказавший что-то
// вслух, обращается к кому-то: к названному по имени либо к единственному
// присутствующему. Без этого реплика уходит в воздух и персонаж не отвечает.
func (s *Session) addressee(in *core.Intent) {
	if in.Args.Target != "" || in.Args.Text == "" {
		return
	}
	if def, ok := core.Verbs[in.Verb]; !ok || def.Class != core.ClassNone {
		return
	}
	var npcs []naming.Candidate
	for _, e := range s.Game.DB.EntitiesAt(s.Game.Node) {
		if e.Kind == store.EntityNPC {
			npcs = append(npcs, naming.Candidate{ID: string(e.ID), Name: e.Name})
		}
	}
	if id, ok := naming.Resolve(in.Args.Text, npcs); ok {
		in.Args.Target = store.EntityID(id)
		return
	}
	if len(npcs) == 1 {
		in.Args.Target = store.EntityID(npcs[0].ID)
	}
}
