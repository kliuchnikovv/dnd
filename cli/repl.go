package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// Session гоняет один и тот же цикл и для интерактивного REPL, и для скрипта:
// прогон воспроизводится парой (seed, файл команд).
type Session struct {
	Game *core.Game
	In   io.Reader
	Out  io.Writer
	r    Render
	sc   *bufio.Scanner
}

func NewSession(g *core.Game, in io.Reader, out io.Writer) *Session {
	return &Session{Game: g, In: in, Out: out}
}

func (s *Session) Run() error {
	fmt.Fprint(s.Out, s.r.Scene(s.Game))
	s.sc = bufio.NewScanner(s.In)
	for s.sc.Scan() {
		cmd, err := Parse(s.sc.Text())
		if err != nil {
			fmt.Fprintf(s.Out, "нельзя: %v\n", err)
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
	case CmdAction:
		cmd.Intent.Actor = g.Actor
		res := g.Apply(cmd.Intent)
		fmt.Fprint(s.Out, r.Turn(g, res))
		if cmd.Intent.Verb == "move_zone" && res.Res != nil && res.Res.Class >= core.OutcomeSuccess {
			g.Node = cmd.Intent.Args.Node
			fmt.Fprint(s.Out, r.Scene(g))
		}
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
