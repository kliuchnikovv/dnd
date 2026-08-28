package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/naming"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
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
	// affordances — печатать ли набор вариантов и разворачивать ли номера.
	// По умолчанию НЕТ: скриптовые прогоны и тесты сверяют вывод дословно, и
	// список, появившийся в них сам, менял бы правду каждого из них разом.
	// Включает его проводка в cmd/dnd — там, где играет человек.
	affordances bool
	// offered — набор, показанный игроку последним. Номер разворачивается по
	// нему, а не пересчитывается: между печатью и вводом состояние не менялось,
	// но пересчёт сделал бы это допущение невидимым.
	offered []core.Affordance
	// optionVoicer — необязательные слова Мастера для набора.
	optionVoicer OptionVoicer
	// offeredWords — слова показанного набора; пусто означает кодовые.
	// voicedKey — отпечаток набора, для которого слова уже спрошены.
	offeredWords []string
	voicedKey    string
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
	// replaying — запись журнала, которую сессия сейчас переигрывает. Не nil
	// означает реплей: журнал в этот момент читают, а не пишут.
	replaying *store.CommandLogEntry
	// journal — журнал действий сессии (ADR-0002). nil означает «не пишем»:
	// проводка флага живёт в cmd/dnd, а игра без журнала обязана работать
	// как работала.
	journal *Journal
	// log — запись игры: читаемый транскрипт прогона. nil означает «не пишем».
	log *GameLog
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

// WithLog включает запись игры. Без неё сессия не пишет ничего, и вывод на
// экран от неё не меняется ни на байт.
func (s *Session) WithLog(l *GameLog) *Session {
	s.log = l
	if l != nil {
		l.note = s.noteOnce
	}
	return s
}

// journalBegin пишет команду до обработки. Поломка журнала не отменяет ход:
// сказать о ней надо, но игра, которая падает из-за надстройки, хуже игры без
// надстройки.
func (s *Session) journalBegin(in core.Intent) store.CommandLogEntry {
	return s.journalBeginForm(in, nil)
}

func (s *Session) journalBeginForm(in core.Intent, form *accusation.Form) store.CommandLogEntry {
	if s.replaying != nil {
		// Реплей журнал читает, а не пишет: команда в нём уже есть, и вторая
		// её копия сделала бы журнал вдвое длиннее прогона. Отметку
		// «применена» ставит journalApplied — она же лечит прерванный ход.
		return *s.replaying
	}
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
	if s.replaying != nil {
		// Аудит живого хода уже записан. Вторая строка сообщала бы, что игрок
		// в этот ход молчал, — а он не молчал, он играл его тогда.
		return
	}
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

// emitEvent — единственная точка, где событие уходит наружу. Одна воронка, а
// не пять вызовов приёмника: запись игры обязана видеть ВСЁ, что видел игрок, а
// вывод, испущенный мимо неё, выпадает из записи молча — и заметить это можно
// только по тому, чего в записи нет.
func (s *Session) emitEvent(e Event) {
	s.sink.Emit(e)
	s.log.event(e)
}

// emit — форматированное событие. Обёртка нужна, чтобы места печати меняли
// только вид события, а не способ вывода.
func (s *Session) emit(kind EventKind, format string, args ...any) {
	s.emitEvent(Event{Kind: kind, Text: fmt.Sprintf(format, args...)})
}

func (s *Session) emitText(kind EventKind, text string) {
	s.emitEvent(Event{Kind: kind, Text: text})
}

// emitSpeech — прямая речь с автором. Точек, где кто-то говорит, несколько
// (NPC, игрок), и у всех должен быть один и тот же
// формат события, иначе полноэкранный режим научится узнавать говорящего
// по месту вызова, а не по данным.
func (s *Session) emitSpeech(speaker, format string, args ...any) {
	s.emitEvent(Event{Kind: EventSpeech, Speaker: speaker, Text: fmt.Sprintf(format, args...)})
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
			s.emitEvent(Event{Kind: EventRefusal, Speaker: MasterName,
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
// WithAffordances включает набор вариантов: печать списка каждый ход и ввод
// номером наравне со словами.
func (s *Session) WithAffordances() *Session {
	s.affordances = true
	return s
}

// WithOptionVoicer отдаёт слова набора Мастеру. Сбой и потолок расхода
// откатывают на кодовые слова: список без слов хуже кодового списка, а
// список без модели обязан работать как работал.
func (s *Session) WithOptionVoicer(v OptionVoicer) *Session {
	s.optionVoicer = v
	return s
}

// OfferedWords — слова показанного набора. Пусто означает, что печатаются
// кодовые: полноэкранная панель обязана видеть ровно то, что напечатал
// построчный режим.
func (s *Session) OfferedWords() []string { return s.offeredWords }

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
	s.offerAffordances()
}

// offerAffordances печатает набор вариантов и запоминает его для разбора
// номера. Одно место на все ветки хода — действие, проба, уточнение, отказ:
// список, появляющийся не после каждого исхода, читался бы как признак того,
// что ход «не тот».
func (s *Session) offerAffordances() {
	if !s.affordances {
		return
	}
	s.offered = s.Game.Affordances(s.spokenTo)
	s.offeredWords = s.voiceOptions(s.offered)
	if text := s.r.Affordances(s.Game, s.offered, s.offeredWords); text != "" {
		s.emitText(EventOptions, text)
	}
}

// voiceOptions просит слова у Мастера. Пустой ответ означает кодовые слова —
// и это законный исход: сбой надстройки не рушит ход.
//
// Слова применяются целиком либо не применяются вовсе. Частичное применение
// подписало бы строку под соседний интент, и игрок, выбравший «поблагодарить»,
// угрожал бы.
func (s *Session) voiceOptions(list []core.Affordance) []string {
	if s.optionVoicer == nil || len(list) == 0 {
		return nil
	}
	key := optionsKey(list)
	if key == s.voicedKey {
		return s.offeredWords
	}
	words, err := s.optionVoicer.VoiceOptions(s.turnContext(), optionsFor(s.Game, list))
	if err != nil {
		s.noteOnce("Мастер не назвал варианты: " + err.Error())
		return nil
	}
	if len(words) != len(list) {
		return nil
	}
	// Ключ запоминается только при успехе. Пометь его раньше — и разовый сбой
	// сети или потолок расхода залип бы до смены набора, а набор в разговоре
	// меняется редко: игрок просидел бы десяток ходов с кодовыми словами из-за
	// одной секундной ошибки.
	s.voicedKey = key
	// Слова — вывод модели по недоверенному вводу, и отвечают они за себя
	// отдельно от разбора: своя строка аудита при той же команде. noteProposal
	// тут не годится: набор предлагается из afterFeed/Start, уже ПОСЛЕ того,
	// как journalAudit отработал за этот ход, а следующий Feed стирает поля
	// предложения в первой же строке. Как и реплика персонажа в speak, пишем
	// в журнал напрямую, минуя очередь разбора.
	s.journal.audit(store.AuditEntry{
		Seq:         s.auditSeq,
		RawInput:    s.raw,
		LLMRole:     string(llm.RoleOptions),
		LLMProposal: llmProposal{Options: words}.encode(),
	})
	return words
}

// Offered — набор, показанный игроку последним.
//
// Структурой, а не текстом: полноэкранный режим рисует его своей панелью и
// подсвечивает выбранный вариант, а разбирать для этого напечатанные строки
// значило бы парсить собственный вывод.
func (s *Session) Offered() []core.Affordance { return s.offered }

// chosen разворачивает номер варианта. Разбирается он ДО структурированного
// парсера: «3» тот отдаёт как неизвестное действие, а перевод свободного текста
// отбивает как бессмыслицу — одна цифра не дотягивает до двух букв.
func (s *Session) chosen(line string) (core.Intent, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || !s.affordances || len(s.offered) == 0 {
		return core.Intent{}, false
	}
	if n < 1 || n > len(s.offered) {
		s.emit(EventRefusal, "такого варианта нет — их %d\n", len(s.offered))
		return core.Intent{}, false
	}
	return s.offered[n-1].Intent, true
}

// chosenToken разворачивает токен опции в её интент. Сервер стоит здесь: токен
// сопоставляется с ПОКАЗАННЫМ набором этого хода, а не декодируется на веру, —
// и найденный интент всё равно уходит в applyIntent на валидацию. Клиент
// интентов не сочиняет; он лишь ссылается на предложенное.
func (s *Session) chosenToken(line string) (core.Intent, bool) {
	if !s.affordances || len(s.offered) == 0 {
		return core.Intent{}, false
	}
	line = strings.TrimSpace(line)
	for _, a := range s.offered {
		if view.OptionToken(a) == line {
			return a.Intent, true
		}
	}
	return core.Intent{}, false
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
	s.log.turn(s.turn)
	s.log.input(line)
	if s.awaitingAccusation() {
		s.feedAccusation(line)
		return s.ended()
	}
	if s.resumeHeld(line) {
		return s.ended()
	}
	// Номер и слова — один путь применения: интент, развёрнутый из варианта,
	// идёт тем же applyIntent, что разобранная фраза. Правда реплея от способа
	// ввода не зависит — в журнал ложится ход, а «3» остаётся в аудите.
	if in, ok := s.chosen(line); ok {
		s.applyIntent(in)
		return s.afterFeed()
	}
	if numeric(line) {
		// Номер вне диапазона: ход не состоялся, и разбирать строку дальше
		// нечего. Отправить «9» в модель значило бы платить за опечатку.
		return s.afterFeed()
	}
	// Токен опции — тот же путь, что номер: клиент шлёт непрозрачный токен,
	// сессия разворачивает его в интент показанного набора и гонит тем же
	// applyIntent. Раньше номера: цифру распознать дешевле, а токен по форме с
	// цифрой не пересекается.
	if in, ok := s.chosenToken(line); ok {
		s.applyIntent(in)
		return s.afterFeed()
	}
	cmd, err := Parse(line)
	if err != nil {
		s.interpret(line, err)
	} else if done := s.dispatch(cmd); done {
		return true
	}
	return s.afterFeed()
}

// afterFeed закрывает ход: сначала развязка, потом набор вариантов. Печатать
// список после раскрытого дела значило бы предлагать ходы в законченной игре.
func (s *Session) afterFeed() bool {
	if s.ended() {
		return true
	}
	s.offerAffordances()
	return false
}

// numeric — строка целиком число. Отдельно от chosen, потому что вопросы
// разные: chosen отвечает «какой это вариант», а этот — «стоит ли вообще
// искать здесь слова».
func numeric(line string) bool {
	_, err := strconv.Atoi(strings.TrimSpace(line))
	return err == nil
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
		s.applyCompare(cmd.Facts)
	case CmdAccuse:
		s.startAccusation()
	case CmdRest:
		s.applyRest(cmd.Text)
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
			s.emitEvent(Event{Kind: EventHunch, Speaker: HunchName,
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
	if in.Args.Target != "" || !utterance(in.Verb) {
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
	if in.Args.Target != "" || in.Args.Text == "" || !utterance(in.Verb) {
		return false
	}
	return len(s.npcsHere()) > 0
}

// utterance — несёт ли ход слова игрока КОМУ-ТО. Раньше здесь стоял класс
// ClassNone, и это было неверно: в него входит look, а осмотреться — не
// обращение.
//
// Признаком речи нельзя считать и непустой Args.Text: переводчик заполняет его
// у любого глагола, потому что слова игрока нужны актёру и при осмотре, и при
// вопросе. Живой прогон под -chat получал на «Осмотреться» вопрос «к кому ты
// обращаешься?» — и ход при этом придерживался, то есть осмотр не происходил
// вовсе.
//
// Набор закрыт и мал сознательно. emote сюда не входит: жест показывают, а не
// произносят, и подставлять ему единственного присутствующего — отдельное
// решение, которого этот список не принимает.
func utterance(v core.Verb) bool { return v == "say" }

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

// applyCompare — сопоставление как ход. Общее тело для команды и реплея: два
// пути к одному ходу разошлись бы, и разошлись бы молча.
//
// Сопоставление выводит факты, то есть меняет состояние, и в журнал идёт
// наравне с действиями: без него реплей разойдётся с прогоном.
func (s *Session) applyCompare(facts []store.FactID) {
	in := core.Intent{Verb: "compare", Args: core.Args{Facts: facts}}
	entry := s.journalBegin(in)
	s.auditSeq = entry.Seq
	res := s.Game.Compare(facts[0], facts[1])
	s.journalApplied(entry)
	s.journalAudit(verdictOf(res))
	s.emitTurn(core.Intent{Verb: "compare"}, res)
}

// applyRest — отдых как ход. Длина лежит в Text: реплею нужно знать, короткий
// он был или длинный.
func (s *Session) applyRest(text string) {
	kind := core.RestShort
	if text == "long" {
		kind = core.RestLong
	}
	if clocks := s.Game.RestPreview(kind); len(clocks) > 0 {
		s.emit(EventSystem, "длинный отдых продвинет часы: %v\n", clocks)
	}
	entry := s.journalBegin(core.Intent{Verb: "rest", Args: core.Args{Text: text}})
	s.auditSeq = entry.Seq
	res := s.Game.Rest(kind)
	s.journalApplied(entry)
	s.journalAudit(verdictOf(res))
	s.emitTurn(core.Intent{Verb: "rest"}, res)
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
