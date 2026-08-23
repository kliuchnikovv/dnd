package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/naming"
	"github.com/kliuchnikovv/dnd/store"
)

// Session гоняет один и тот же цикл и для интерактивного REPL, и для скрипта:
// прогон воспроизводится парой (seed, файл команд).
type Session struct {
	Game *core.Game
	In   io.Reader
	// sink — куда уходит вывод. Сессия печатает только через него: иначе
	// полноэкранный режим пришлось бы делать вторым форматом вывода, а не
	// вторым приёмником.
	sink   Sink
	r      Render
	sc     *bufio.Scanner
	interp Interpreter
	// chat — переводчик чат-режима. Взаимоисключен с interp: два разбора
	// одной фразы это два разных ответа на один ввод.
	chat     ChatInterpreter
	voicer   Voicer
	narrator Narrator
	// refuse — голос Мастера у отказа мира. Пустой означает прежнее «нельзя:
	// …» побайтово: игра без моделей обязана работать как работала.
	refuse func(string) string
	// noted — о какой поломке надстройки уже сказано. Жаловаться на неё
	// каждой строкой значит топить в шуме сам вывод игры.
	noted map[string]bool
	turn  int
	// spokenTo — последний, к кому обращались. Разговор продолжается с тем же
	// человеком: игроку не надо называть его в каждой реплике.
	spokenTo store.EntityID
	// held — ход, отложенный вопросом «к кому ты обращаешься?». Ответ на этот
	// вопрос ДОГОВАРИВАЕТ начатое, а не начинает новое: без этого фраза игрока
	// исчезала, а названное имя уезжало в движок отдельным обращением — живой
	// прогон получал «вы заводите разговор о погоде» вместо своего вопроса.
	held *core.Intent
	// pending — вопрос, заданный игроку игрой. Следующая его фраза — ответ на
	// этот вопрос, и разбор обязан это знать.
	pending string
	// hunch — показывать ли подсказки чутья. По умолчанию НЕТ: живой прогон
	// показал, что чутьё срабатывает, пока игрок спокойно осматривает мир —
	// счётчик холостых ходов считает любой ход без находки, а осмотр без
	// находки это нормальный осмотр, а не «встал». Пока признак «застрял» не
	// стал честнее, помощь молчит: подсказка не вовремя хуже её отсутствия,
	// потому что читает решение вслух.
	hunch bool
	// chatShown, chatEaten — сколько реплик Мастера игрок увидел и сколько
	// съел отказ ядра. Эксперимент надо мерить: высокая доля съеденных значит,
	// что модель уверенно отвечает на ходы, которых мир не допускает, — и
	// тогда порядок «сначала ответить» неверен.
	chatShown, chatEaten int
	// raw, llmRole, llmProposal — аудит текущего хода: сырой ввод игрока и то,
	// что предложила по нему модель (ADR-0002). Живут ровно один ход: аудит
	// пишется рядом с командой, а не накапливается.
	raw         string
	llmRole     string
	llmProposal string
	// auditSeq — команда текущего хода. Реплика персонажа приходит уже после
	// применения, и ей нужно, к чему приписаться. Ноль означает, что команды
	// не было.
	auditSeq int
	// journal — журнал действий сессии (ADR-0002). nil означает «не пишем»:
	// проводка флага живёт в cmd/dnd, а игра без журнала обязана работать
	// как работала.
	journal *Journal
	// accusing — открытый набор слотов обвинения. Не nil, пока сессия ждёт
	// очередной токен: полноэкранный режим отдаёт ввод по одной строке и
	// не может сам дождаться следующей внутри одного хода.
	accusing *accusation4
}

func NewSession(g *core.Game, in io.Reader, out io.Writer) *Session {
	return &Session{Game: g, In: in, sink: TextSink{W: out}}
}

// WithSink подменяет приёмник вывода.
func (s *Session) WithSink(k Sink) *Session {
	s.sink = k
	return s
}

// WithJournal включает журнал действий: команда пишется до обработки ядром и
// отмечается применённой после. Без него сессия не пишет ничего.
func (s *Session) WithJournal(j *Journal) *Session {
	s.journal = j
	return s
}

// journalBegin пишет команду до обработки. Поломка журнала не отменяет ход:
// сказать о ней надо, но игра, которая падает из-за надстройки, хуже игры без
// надстройки.
func (s *Session) journalBegin(in core.Intent) store.CommandLogEntry {
	return s.journalBeginForm(in, nil)
}

func (s *Session) journalBeginForm(in core.Intent, form *accusation.Form) store.CommandLogEntry {
	e, err := s.journal.begin(in, form, s.turn)
	if err != nil {
		s.noteOnce("журнал: " + err.Error())
	}
	return e
}

// journalApplied отмечает команду применённой после возврата из ядра.
func (s *Session) journalApplied(e store.CommandLogEntry) {
	if err := s.journal.commit(e); err != nil {
		s.noteOnce("журнал: " + err.Error())
	}
}

// journalAudit пишет строку аудита текущего хода: сырой ввод игрока,
// предложение модели, вердикт ядра. Отвечает на единственный вопрос, на
// который журнал команд не отвечает: что предлагали против того, что применили.
func (s *Session) journalAudit(verdict string) {
	s.journal.audit(store.AuditEntry{
		Seq:         s.auditSeq,
		RawInput:    s.raw,
		LLMRole:     s.llmRole,
		LLMProposal: s.llmProposal,
		CoreVerdict: verdict,
	})
}

// noteProposal запоминает, что предложила модель по этому вводу. Вердикта у неё
// нет: его выносит ядро, и в одной строке они встречаются позже.
func (s *Session) noteProposal(role llm.Role, p llmProposal) {
	s.llmRole, s.llmProposal = string(role), p.encode()
}

// emit — форматированное событие. Обёртка нужна, чтобы места печати меняли
// только вид события, а не способ вывода.
func (s *Session) emit(kind EventKind, format string, args ...any) {
	s.sink.Emit(Event{Kind: kind, Text: fmt.Sprintf(format, args...)})
}

func (s *Session) emitText(kind EventKind, text string) {
	s.sink.Emit(Event{Kind: kind, Text: text})
}

// emitSpeech — прямая речь с автором. Точек, где кто-то говорит, несколько
// (NPC, игрок), и у всех должен быть один и тот же
// формат события, иначе полноэкранный режим научится узнавать говорящего
// по месту вызова, а не по данным.
func (s *Session) emitSpeech(speaker, format string, args ...any) {
	s.sink.Emit(Event{Kind: EventSpeech, Speaker: speaker, Text: fmt.Sprintf(format, args...)})
}

// WithRefusalVoice отдаёт отказ мира Мастеру. Формулировку решает не он:
// текст отказа приходит из ядра, Мастер только одевает его в речь — иначе у
// «можно» появилось бы второе место правды. Голос, вернувший пустую строку
// (сбой модели), означает откат на прежний вывод.
func (s *Session) WithRefusalVoice(v func(string) string) *Session {
	s.refuse = v
	return s
}

// turnNotSpent — механический хвост отказа. Печатается КОДОМ, а не голосом:
// «ход не потрачен» это факт движка, и отдавать его прозе значит позволить ей
// об этом врать. Без него живая формулировка съедает единственный признак, по
// которому игрок отличает отказ от провала.
const turnNotSpent = "  (ход не потрачен)\n"

// emitTurn печатает исход хода. Отказ уходит своим путём: у него есть автор,
// и он единственный, кого игра отдаёт Мастеру на переформулировку.
func (s *Session) emitTurn(in core.Intent, res core.TurnResult) {
	if res.Refused {
		s.emitRefusal(res.Refusal)
		return
	}
	s.emitText(EventProse, s.r.Turn(s.Game, in, res))
}

// emitRefusal — отказ мира. Одно место на всю игру: отказы приходят из трёх
// команд, и расхождение между ними было бы багом, видимым только в одной.
func (s *Session) emitRefusal(refusal string) {
	if s.refuse != nil {
		if said := strings.TrimSpace(s.refuse(refusal)); said != "" {
			s.sink.Emit(Event{Kind: EventRefusal, Speaker: MasterName,
				Text: said + "\n" + turnNotSpent})
			return
		}
	}
	s.emit(EventRefusal, "нельзя: %s\n", refusal)
}

// WithHunch включает подсказки чутья. Выключены по умолчанию — см. поле hunch.
func (s *Session) WithHunch() *Session {
	s.hunch = true
	return s
}

// ChatStats — сколько реплик чат-режима показано и сколько съедено отказом
// ядра. Нули означают, что чат-режим не работал.
func (s *Session) ChatStats() (shown, eaten int) { return s.chatShown, s.chatEaten }

// Start печатает стартовую сцену. Отдельно от Run, потому что драйверов два:
// построчный читает stdin сам, полноэкранный владеет своим циклом.
func (s *Session) Start() {
	// Брифинг — голос Мастера, список известного — печать кода. Разделены по
	// той же причине, по которой бросок Мастеру не принадлежит: прозе нельзя
	// доверять точность, а брифинг это единственное место, где игрок обязан
	// получить факты дела дословно.
	if text := s.r.Briefing(s.Game); text != "" {
		s.emitText(EventProse, text)
	}
	if known := s.r.Known(s.Game); known != "" {
		s.emitText(EventSystem, known)
	}
	s.emitText(EventScene, s.r.Scene(s.Game))
}

// Feed исполняет ОДИН ввод и сообщает, пора ли заканчивать. Здесь живёт всё,
// что раньше было телом цикла Run: разбор, перевод свободного текста,
// исполнение и проверка развязки.
func (s *Session) Feed(line string) bool {
	s.turn++
	// Сырой ввод — правда аудита, но НЕ правда реплея: переигрывать текст
	// через модель нельзя, второй прогон даст другой интент. Реплей берёт
	// интент из журнала команд, разбор инцидента — эту строку.
	s.raw, s.llmRole, s.llmProposal, s.auditSeq = line, "", "", 0
	if s.awaitingAccusation() {
		s.feedAccusation(line)
		return s.ended()
	}
	if s.resumeHeld(line) {
		return s.ended()
	}
	cmd, err := Parse(line)
	if err != nil {
		s.interpret(line, err)
	} else if done := s.dispatch(cmd); done {
		return true
	}
	return s.ended()
}

// resumeHeld договаривает ход, отложенный вопросом «к кому?». Отложенный ход
// отпускается в любом случае: если игрок передумал и написал другое, запирать
// его в вопросе нельзя — ввод пойдёт обычным путём как новый.
func (s *Session) resumeHeld(line string) bool {
	held := s.held
	if held == nil {
		return false
	}
	s.held = nil
	id, ok := naming.Resolve(line, s.npcsHere())
	if !ok {
		return false
	}
	held.Args.Target = store.EntityID(id)
	s.execute(*held)
	return true
}

func (s *Session) Run() error {
	s.Start()
	s.sc = bufio.NewScanner(s.In)
	if s.ended() {
		return nil
	}
	for s.sc.Scan() {
		if s.Feed(s.sc.Text()) {
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
	case CmdItems:
		s.emitText(EventSystem, r.Items(g))
	case CmdClocks:
		s.emitText(EventSystem, r.Clocks(g))
	case CmdCompare:
		// Сопоставление выводит факты, то есть меняет состояние, и в журнал
		// идёт наравне с действиями: без него реплей разойдётся с прогоном.
		in := core.Intent{Verb: "compare", Args: core.Args{Facts: cmd.Facts}}
		entry := s.journalBegin(in)
		s.auditSeq = entry.Seq
		res := g.Compare(cmd.Facts[0], cmd.Facts[1])
		s.journalApplied(entry)
		s.journalAudit(verdictOf(res))
		s.emitTurn(core.Intent{Verb: "compare"}, res)
	case CmdAccuse:
		s.startAccusation()
	case CmdRest:
		kind := core.RestShort
		if cmd.Text == "long" {
			kind = core.RestLong
		}
		if clocks := g.RestPreview(kind); len(clocks) > 0 {
			s.emit(EventSystem, "длинный отдых продвинет часы: %v\n", clocks)
		}
		// Отдых двигает часы — ход, меняющий состояние. Длина отдыха лежит в
		// Text: реплею нужен короткий он был или длинный.
		entry := s.journalBegin(core.Intent{Verb: "rest", Args: core.Args{Text: cmd.Text}})
		s.auditSeq = entry.Seq
		res := g.Rest(kind)
		s.journalApplied(entry)
		s.journalAudit(verdictOf(res))
		s.emitTurn(core.Intent{Verb: "rest"}, res)
	case CmdAction:
		s.applyIntentWithHint(cmd.Intent, cmd.Text)
	}
	return false
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
	// Чутьё вступает, когда расследование встало. Текст авторский, момент
	// выбирает движок: подсказка на каждом ходу читает решение вслух, а
	// подсказка никогда — это закрытая консоль на первой сессии.
	//
	// Автор события — само чутьё, а не персонаж. Раньше подсказку произносил
	// напарник и оказывался в ответе за слова, которые выбрал движок: четыре
	// подсказки «Гавани» из шести называют человека или запись, то есть ровно
	// то, что гвард обязан рубить как выдумку персонажа.
	if s.hunch {
		if line, ok := s.Game.Hint(); ok {
			s.sink.Emit(Event{Kind: EventHunch, Speaker: HunchName,
				Text: HunchMark + line + "\n"})
		}
	}
	if in.Verb == "move_zone" && res.Res != nil && res.Res.Class >= core.OutcomePartial {
		s.Game.MoveTo(in.Args.Node)
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
	ready, ok := s.prepare(in, hint)
	if !ok {
		return
	}
	s.execute(ready)
}

// prepare доводит интент до исполнимого вида: актёр, адресат, вопрос игроку,
// если адресата взять негде. Отделено от исполнения ради чат-режима: там надо
// знать, что ход состоится, ДО того как игрок увидит реплику Мастера, — а
// вопрос «к кому ты обращаешься» означает, что не состоится.
//
// false означает, что ход не пойдёт: игре есть что спросить.
func (s *Session) prepare(in core.Intent, hint string) (core.Intent, bool) {
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
		// Ход придержан: ответ на вопрос его договорит. Иначе сказанное
		// исчезает, и игрок отвечает на вопрос игры в пустоту.
		s.held = &in
		return in, false
	}
	return in, true
}

// execute — единственное место, где интент доходит до движка. И команда, и
// свободный текст, и чат-режим идут через него: расхождение между входами
// было бы багом, который проявляется только в одном из режимов.
func (s *Session) execute(in core.Intent) {
	s.remember(in)
	// Запись до обработки: команда лежит в журнале раньше, чем ядро тронуло
	// состояние. Падение между этими двумя строками — единственный случай,
	// который реплей хвоста pending обязан вылечить.
	entry := s.journalBegin(in)
	s.auditSeq = entry.Seq
	res := s.Game.Apply(in)
	s.journalApplied(entry)
	s.journalAudit(verdictOf(res))
	if said := spokenAloud(in); said != "" && !res.Refused {
		// Реплика игрока показывается как реплика. Описание того, что он
		// «сказал это вслух», на каждой фразе читается как шум.
		s.emitSpeech(PlayerName, "%s\n", Spoken(said))
	} else {
		s.emitTurn(in, res)
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
