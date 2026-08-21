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
	Game *core.Game
	In   io.Reader
	Out  io.Writer
	// sink — куда уходит вывод. Сессия печатает только через него: иначе
	// полноэкранный режим пришлось бы делать вторым форматом вывода, а не
	// вторым приёмником.
	sink     Sink
	r        Render
	sc       *bufio.Scanner
	interp   Interpreter
	voicer   Voicer
	narrator Narrator
	// noted — о какой поломке надстройки уже сказано. Жаловаться на неё
	// каждой строкой значит топить в шуме сам вывод игры.
	noted map[string]bool
	turn  int
	// spokenTo — последний, к кому обращались. Разговор продолжается с тем же
	// человеком: игроку не надо называть его в каждой реплике.
	spokenTo store.EntityID
	// pending — вопрос, заданный игроку игрой. Следующая его фраза — ответ на
	// этот вопрос, и разбор обязан это знать.
	pending string
}

func NewSession(g *core.Game, in io.Reader, out io.Writer) *Session {
	return &Session{Game: g, In: in, Out: out, sink: TextSink{W: out}}
}

// WithSink подменяет приёмник вывода.
func (s *Session) WithSink(k Sink) *Session {
	s.sink = k
	return s
}

// emit — форматированное событие. Обёртка нужна, чтобы места печати меняли
// только вид события, а не способ вывода.
func (s *Session) emit(kind EventKind, format string, args ...any) {
	s.sink.Emit(Event{Kind: kind, Text: fmt.Sprintf(format, args...)})
}

func (s *Session) emitText(kind EventKind, text string) {
	s.sink.Emit(Event{Kind: kind, Text: text})
}

func (s *Session) Run() error {
	s.emitText(EventScene, s.r.Scene(s.Game))
	s.sc = bufio.NewScanner(s.In)
	if s.ended() {
		return nil
	}
	for s.sc.Scan() {
		line := s.sc.Text()
		s.turn++
		cmd, err := Parse(line)
		if err != nil {
			s.interpret(line, err)
		} else if done := s.dispatch(cmd); done {
			return nil
		}
		if s.ended() {
			return nil
		}
	}
	return s.sc.Err()
}

// ended закрывает прогон развязкой. Раскрытое дело уже напечатало речь игрока
// и последствия; висяку остаётся напечатать свой текст. Продолжать после
// любого из двух исходов нечего.
func (s *Session) ended() bool {
	if s.Game.Solved() {
		return true
	}
	if !s.Game.Stalled() {
		return false
	}
	if cold := s.Game.ColdCase(); cold != "" {
		s.emit(EventSystem, "\n%s\n", cold)
	}
	return true
}

func (s *Session) dispatch(cmd Command) bool {
	g, r := s.Game, s.r
	switch cmd.Kind {
	case CmdNone:
	case CmdQuit:
		return true
	case CmdHelp:
		s.emitText(EventSystem, r.Help())
	case CmdSurvey:
		s.emitText(EventSystem, r.Survey(g))
	case CmdFacts:
		s.emitText(EventSystem, r.Facts(g))
	case CmdState:
		s.emitText(EventSystem, r.State(g))
	case CmdClocks:
		s.emitText(EventSystem, r.Clocks(g))
	case CmdCompare:
		s.emitText(EventProse, r.Turn(g, core.Intent{Verb: "compare"}, g.Compare(cmd.Facts[0], cmd.Facts[1])))
	case CmdAccuse:
		s.accuse()
	case CmdRest:
		kind := core.RestShort
		if cmd.Text == "long" {
			kind = core.RestLong
		}
		if clocks := g.RestPreview(kind); len(clocks) > 0 {
			s.emit(EventSystem, "длинный отдых продвинет часы: %v\n", clocks)
		}
		s.emitText(EventProse, r.Turn(g, core.Intent{Verb: "rest"}, g.Rest(kind)))
	case CmdAction:
		s.applyIntentWithHint(cmd.Intent, cmd.Text)
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
		fmt.Fprintf(s.Out, "Обвинение верно. Попыток: %d.\n\n", res.Attempt)
		// Развязка — речь игрока, собранная из его же выводов. Игра здесь
		// ничего не объясняет: она повторяет то, что он собрал сам.
		for _, clause := range res.Summation {
			fmt.Fprintf(s.Out, "  — %s\n", clause)
		}
		if after := g.Aftermath(); after != "" {
			fmt.Fprintf(s.Out, "\n%s\n", after)
		}
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
	// Напарник вступает, когда расследование встало. Реплика авторская, момент
	// выбирает движок: подсказка на каждом ходу читает решение вслух, а
	// подсказка никогда — это закрытая консоль на первой сессии.
	if line, ok := s.Game.Hint(); ok {
		if e, found := s.Game.DB.Entities[s.Game.Companion]; found {
			s.sink.Emit(Event{Kind: EventSpeech, Speaker: e.Name, Text: fmt.Sprintf("%s: %s\n", e.Name, line)})
		}
	}
	if in.Verb == "move_zone" && res.Res != nil && res.Res.Class >= core.OutcomePartial {
		s.Game.Node = in.Args.Node
		s.emitText(EventScene, s.r.Scene(s.Game))
	}
}

// addressee подставляет собеседника прямой речи, перебирая три способа от
// надёжного к правдоподобному: назван по имени, разговор уже идёт с ним,
// он единственный в сцене. Без адресата реплика уходит в воздух — и это
// худший исход, потому что игрок не понимает, сработало ли что-нибудь.
func (s *Session) addressee(in *core.Intent, hint string) {
	if in.Args.Target != "" {
		return
	}
	if def, ok := core.Verbs[in.Verb]; !ok || def.Class != core.ClassNone {
		return
	}
	npcs := s.npcsHere()
	// Подсказка вне кавычек надёжнее самой реплики: в ней игрок как раз и
	// называет, к кому обращается.
	for _, source := range []string{hint, in.Args.Text} {
		if source == "" {
			continue
		}
		if id, ok := naming.Resolve(source, npcs); ok {
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
func (s *Session) applyIntent(in core.Intent) { s.applyIntentWithHint(in, "") }

// applyIntentWithHint принимает подсказку об адресате из той части строки,
// что осталась вне кавычек: «Обращаясь к Нильсу» стоит именно там.
func (s *Session) applyIntentWithHint(in core.Intent, hint string) {
	in.Actor = s.Game.Actor
	s.addressee(&in, hint)
	if s.needsAddressee(in) {
		// Спросить дешевле, чем промолчать: молчание игрок читает как
		// поломку, а не как отсутствие адресата.
		var names []string
		for _, c := range s.npcsHere() {
			names = append(names, c.Name)
		}
		s.emit(EventPrompt, "к кому ты обращаешься? здесь %s\n", strings.Join(names, ", "))
		return
	}
	s.remember(in)
	res := s.Game.Apply(in)
	if said := spokenAloud(in); said != "" && !res.Refused {
		// Реплика игрока показывается как реплика. Описание того, что он
		// «сказал это вслух», на каждой фразе читается как шум.
		s.emit(EventSystem, "%s\n", Spoken(said))
	} else {
		s.emitText(EventProse, s.r.Turn(s.Game, in, res))
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
