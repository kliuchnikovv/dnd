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
	// spokenTo — последний, к кому обращались. Разговор продолжается с тем же
	// человеком: игроку не надо называть его в каждой реплике.
	spokenTo store.EntityID
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
		s.applyIntent(cmd.Intent)
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

// addressee подставляет собеседника прямой речи, перебирая три способа от
// надёжного к правдоподобному: назван по имени, разговор уже идёт с ним,
// он единственный в сцене. Без адресата реплика уходит в воздух — и это
// худший исход, потому что игрок не понимает, сработало ли что-нибудь.
func (s *Session) addressee(in *core.Intent) {
	if in.Args.Target != "" {
		return
	}
	if def, ok := core.Verbs[in.Verb]; !ok || def.Class != core.ClassNone {
		return
	}
	npcs := s.npcsHere()
	if in.Args.Text != "" {
		if id, ok := naming.Resolve(in.Args.Text, npcs); ok {
			in.Args.Target = store.EntityID(id)
			return
		}
	}
	if s.spokenTo != "" && containsID(npcs, string(s.spokenTo)) {
		in.Args.Target = s.spokenTo
		return
	}
	if len(npcs) == 1 {
		in.Args.Target = store.EntityID(npcs[0].ID)
	}
}

// needsAddressee сообщает, что сказанное некому услышать. Тогда надо спросить,
// а не промолчать.
func (s *Session) needsAddressee(in core.Intent) bool {
	if in.Args.Target != "" || in.Args.Text == "" {
		return false
	}
	def, ok := core.Verbs[in.Verb]
	return ok && def.Class == core.ClassNone && len(s.npcsHere()) > 0
}

func (s *Session) npcsHere() []naming.Candidate {
	var out []naming.Candidate
	for _, e := range s.Game.DB.EntitiesAt(s.Game.Node) {
		if e.Kind == store.EntityNPC {
			out = append(out, naming.Candidate{ID: string(e.ID), Name: e.Name})
		}
	}
	return out
}

func containsID(list []naming.Candidate, id string) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}

// remember запоминает собеседника, чтобы следующая реплика ушла ему же.
func (s *Session) remember(in core.Intent) {
	if in.Args.Target == "" {
		return
	}
	if e, ok := s.Game.DB.Entities[in.Args.Target]; ok && e.Kind == store.EntityNPC {
		s.spokenTo = in.Args.Target
	}
}

// applyIntent — единственный путь, которым интент доходит до движка. И
// команда, и свободный текст идут через него: расхождение между двумя входами
// было бы багом, который проявляется только в одном из режимов.
func (s *Session) applyIntent(in core.Intent) {
	in.Actor = s.Game.Actor
	s.addressee(&in)
	if s.needsAddressee(in) {
		// Спросить дешевле, чем промолчать: молчание игрок читает как
		// поломку, а не как отсутствие адресата.
		var names []string
		for _, c := range s.npcsHere() {
			names = append(names, c.Name)
		}
		fmt.Fprintf(s.Out, "к кому ты обращаешься? здесь %s\n", strings.Join(names, ", "))
		return
	}
	s.remember(in)
	res := s.Game.Apply(in)
	if said := spokenAloud(in); said != "" && !res.Refused {
		// Реплика игрока показывается как реплика. Описание того, что он
		// «сказал это вслух», на каждой фразе читается как шум.
		fmt.Fprintln(s.Out, Spoken(said))
	} else {
		fmt.Fprint(s.Out, s.r.Turn(s.Game, res))
	}
	s.afterAction(in, res)
}

// spokenAloud возвращает сказанное игроком вслух, если ход этим и был.
func spokenAloud(in core.Intent) string {
	if in.Verb != "say" {
		return ""
	}
	return strings.TrimSpace(in.Args.Text)
}
