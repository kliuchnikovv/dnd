package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
)

// debugTitle — шапка панели отладки.
const debugTitle = "— обмен с моделью · Tab возвращает к разговору —"

// Options — что полноэкранному режиму нужно знать о запуске. Status отдаёт
// строку шапки (расход, ход): собирать её здесь значило бы тянуть в интерфейс
// слой моделей, который к экрану отношения не имеет.
type Options struct {
	Title  string
	Status func() string
	Debug  *Ring
	// NoDebugReason — что показать в панели отладки, если полного дампа в
	// ней нет: либо Debug == nil совсем, либо кольцо есть (как приёмник
	// алерта леджера и «Мастер не ответил»), но пусто, потому что дамп не
	// просили. Пусто само — берётся дефолт «запустите с -debug-llm»; своя
	// причина нужна, когда он не подходит (см. main.noDebugReasonFor).
	NoDebugReason string
}

// eventMsg — событие игры, дошедшее до интерфейса.
type eventMsg struct{ event cli.Event }

// doneMsg — ход закончился; true означает, что игра закончена.
type doneMsg struct{ quit bool }

// startedMsg — s.Start() отработал. До этого момента ввод заблокирован тем
// же busy, что и обычный ход: без этого Enter, нажатый раньше, чем стартовая
// сцена допечаталась, запускает s.Feed параллельно с ещё живым s.Start —
// cli.Session не рассчитан на параллельные вызовы и не блокируется сам,
// так что turn/Game/pending/spokenTo разъедутся непредсказуемо.
type startedMsg struct{}

type model struct {
	session *cli.Session
	opts    Options

	transcript Transcript
	view       viewport.Model
	input      textinput.Model
	history    History

	events chan cli.Event
	// done закрывается ровно один раз при выходе (Ctrl-C или конец игры) и
	// служит вторым исходом для sink.Emit: пока цикл программы жив, запись
	// в events блокируется как положено, а после выхода — просто отпускает
	// пишущую горутину, вместо того чтобы держать её вечно на закрытом всеми
	// читателями канале.
	done chan struct{}
	// busy — ход (или стартовая сцена) исполняется. Ввод в это время
	// заблокирован: два хода одновременно ядро не переживёт, а очередь фраз
	// перепутает ответы.
	busy bool
	// ended — игра закончена (доигранное дело или висяк), но программа ещё
	// не вышла: развязку надо показать и дать её прочитать, а не гасить
	// альт-экран в момент, когда s.Feed вернул true. Любая клавиша (кроме
	// уже перехваченного Ctrl-C) из этого состояния ведёт в quit().
	ended bool
	// lastKind — вид последнего дошедшего события. Нужен ровно для одного
	// решения: пустой Enter — валидный ответ, когда игра только что спросила
	// (слот обвинения, уточняющий вопрос), и пустой в остальное время —
	// потому что EventPrompt и есть тот сигнал ожидания ввода, который cli
	// публично не отдаёт.
	lastKind cli.EventKind
	// prompt — текст последнего EventPrompt, показанный подсказкой у строки
	// ввода. В транскрипт приглашение не льётся: спека §2.1 требует его у
	// поля ввода, а не построчным блоком вперемешку с речью.
	prompt string
	// offered — набор вариантов, показанный панелью у строки ввода, и selected
	// — курсор по нему. Копия из сессии, а не ссылка: модель bubbletea
	// копируется на каждое сообщение, и делить срез с горутиной хода нельзя.
	offered  []core.Affordance
	selected int
	// history — курсор истории ввода. Стрелки заняты выбором варианта, и
	// история переехала на Ctrl-P/Ctrl-N: варианты нужны каждый ход, история —
	// изредка, а частый жест обязан быть проще.
	debugOpen bool
	quitting  bool
	width     int
	height    int
}

func newModel(s *cli.Session, opts Options) model {
	in := textinput.New()
	in.Prompt = "› "
	in.Focus()
	return model{
		session: s, opts: opts,
		input:  in,
		view:   viewport.New(80, 20),
		events: make(chan cli.Event, 64),
		done:   make(chan struct{}),
		// busy взведён с самого начала: ниже он снимается только после
		// startedMsg, то есть после того, как стартовая сцена реально
		// допечатана — см. комментарий у startedMsg.
		busy: true,
		// Ширина по умолчанию — до первого tea.WindowSizeMsg окна ещё не
		// знает своего размера, а рендерить транскрипт по нулевой ширине
		// незачем: разделитель схлопнется в мусор. 80 — тот же дефолт, что
		// у viewport.New ниже.
		width: 80,
		// Высота по той же причине, что ширина: события стартовой сцены могут
		// прийти раньше первого tea.WindowSizeMsg, и расчёт раскладки по нулю
		// оставил бы транскрипту одну строку.
		height: 24,
		// Ни один вариант не подсвечен: выбор делается осознанно, а
		// подсвеченная сама собой первая строка означала бы, что Enter на
		// пустой строке исполняет ход, которого игрок не выбирал.
		selected: noSelection,
	}
}

// Init — в брифе тут был textinput.Blink, которого в установленной версии
// bubbles больше нет: мигание курсора отдано отдельному пакету cursor, и
// команда живёт как cursor.Blink. Ведёт себя так же — курсор мигает, пока
// поле в фокусе.
func (m model) Init() tea.Cmd { return tea.Batch(cursor.Blink, m.start()) }

// start печатает стартовую сцену через тот же приёмник, что и ходы. Msg —
// startedMsg, а не сразу событие: слушать канал начинаем в Update после
// того, как busy снят, иначе первый Enter может обогнать s.Start().
func (m model) start() tea.Cmd {
	return func() tea.Msg {
		if m.session != nil {
			m.session.Start()
		}
		return startedMsg{}
	}
}

// waitEvent подписывается на следующее событие. Ход идёт в своей горутине и
// шлёт события по мере готовности — реплика появляется, когда готова, а не
// пачкой в конце.
func waitEvent(ch chan cli.Event) tea.Cmd {
	return func() tea.Msg { return eventMsg{<-ch} }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.view.Width = msg.Width
		m.input.Width = msg.Width - 4
		m.refresh()
		return m, nil

	case startedMsg:
		// Стартовая сцена реально допечатана — только теперь снимаем busy
		// и начинаем слушать канал. До этой строки Enter был заблокирован
		// тем же busy, что и обычный ход.
		m.busy = false
		return m, waitEvent(m.events)

	case eventMsg:
		m.lastKind = msg.event.Kind
		if msg.event.Kind == cli.EventOptions {
			// Набор — панель у строки ввода, а не строка транскрипта: он себя
			// заменяет каждый ход, и в потоке копился бы устаревшими копиями.
			m.takeOptions()
			m.refresh()
			return m, waitEvent(m.events)
		}
		if msg.event.Kind == cli.EventPrompt {
			// Приглашение — не строка транскрипта, а подсказка у ввода: у
			// EventPrompt нет автора (speakerOf отдаёт пустую строку), и
			// блоком в общем потоке оно печаталось бы безымянным — вперемешку
			// с речью, да ещё и с хвостом построчного курсора "> ", который
			// в полноэкранном виде не нужен.
			m.prompt = promptText(msg.event.Text)
		} else {
			m.prompt = ""
			m.transcript.Append(msg.event)
		}
		m.refresh()
		// Пока открыта панель отладки, событие транскрипта не должно
		// прокручивать её к концу — там читают дамп, а не следят за ходом.
		if !m.debugOpen {
			m.view.GotoBottom()
		}
		return m, waitEvent(m.events)

	case doneMsg:
		m.busy = false
		if msg.quit {
			// Развязку показывают, а не гасят: постановление контроллера —
			// не выходить тут же в tea.Quit. Оставшиеся в канале события
			// (клаузы саммации, последствия, текст висяка) дочитывает та же
			// цепочка waitEvent, что уже запущена — она не останавливалась.
			// Программа реально закроется по любой клавише из key().
			m.ended = true
			return m, nil
		}
		return m, nil

	case tea.KeyMsg:
		return m.key(msg)

	case tea.MouseMsg:
		// Колесо крутит транскрипт. Без этого его крутит сам терминал — и
		// показывает свой скроллбек: игрок, потянувшийся к истории разговора,
		// видит историю шелла. Дальше строки ввода событие не идёт: колесу
		// там делать нечего, а default-ветка отдала бы его именно ей.
		m.view, _ = m.view.Update(msg)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl-C выходит сразу и из любого состояния, включая уже показанную
	// развязку — так было в брифе, и это сохранено. Не сохранено только
	// слепое доверие к тому, что после tea.Quit больше некому дочитать
	// events: за это отвечает quit(), закрывая done.
	if msg.Type == tea.KeyCtrlC {
		return m.quit()
	}
	if m.ended {
		// Развязка уже на экране (клаузы, последствия либо текст висяка) —
		// любая другая клавиша теперь просто закрывает окно.
		return m.quit()
	}
	switch msg.Type {
	case tea.KeyTab:
		m.debugOpen = !m.debugOpen
		m.refresh()
		return m, nil
	case tea.KeyPgUp, tea.KeyCtrlB:
		m.view.HalfViewUp()
		return m, nil
	case tea.KeyPgDown, tea.KeyCtrlF:
		m.view.HalfViewDown()
		return m, nil
	case tea.KeyUp:
		m.selectionUp()
		return m, nil
	case tea.KeyDown:
		m.selectionDown()
		return m, nil
	case tea.KeyCtrlP:
		if line, ok := m.history.Prev(); ok {
			m.input.SetValue(line)
			m.input.CursorEnd()
		}
		return m, nil
	case tea.KeyCtrlN:
		line, _ := m.history.Next()
		m.input.SetValue(line)
		m.input.CursorEnd()
		return m, nil
	case tea.KeyEnter:
		if m.busy {
			return m, nil
		}
		line := strings.TrimSpace(m.input.Value())
		// Выбранный стрелками вариант исполняется Enter — но только на пустой
		// строке. Набранная фраза старше выбора: игрок, начавший печатать,
		// передумал, и исполнить вместо его слов подсвеченную строку значило бы
		// проглотить набранное.
		echo := ""
		if line == "" {
			if chosen, ok := m.chosenLine(); ok {
				line, echo = chosen, m.chosenEcho()
			}
		}
		// Пустая строка в построчном режиме — валидный токен слота
		// обвинения (и валидный ответ на уточняющий вопрос): sc.Scan() там
		// отдаёт пустые строки как есть. Полноэкранный режим обязан вести
		// себя так же. cli не отдаёт публичного признака «жду ответ на
		// prompt», поэтому берём то, что уже есть у модели: последнее
		// событие было EventPrompt — значит игра только что спросила.
		if line == "" && m.lastKind != cli.EventPrompt {
			return m, nil
		}
		// Выбранный вариант в историю не идёт: «3» прошлого хода на следующем
		// означает другой ход, и подставленная стрелкой цифра исполнила бы не
		// то, что игрок помнит.
		if echo == "" {
			m.history.Add(line)
		}
		if line != "" {
			// Своя строка — часть разговора, и встать она обязана ДО того,
			// как ход что-то ответит: иначе игрок не видит, на что отвечают.
			// Пустая строка (валидный токен слота обвинения) эхом даёт
			// подпись без слов, поэтому не печатается.
			//
			// За выбранный вариант печатаются СЛОВА, а не номер: игрок выбрал
			// ход, а не нажал «3». Сырой ввод для аудита при этом остаётся
			// номером — это разные правды (ADR-0002).
			shown := line
			if echo != "" {
				shown = echo
			}
			m.transcript.AppendInput(shown)
		}
		m.input.SetValue("")
		m.busy = true
		// Ответ ушёл — приглашение больше не актуально. Если ход задаст
		// следующий слот, eventMsg выставит новое приглашение сам.
		m.prompt = ""
		m.refresh()
		// Та же оговорка, что у eventMsg: пока открыта панель отладки, своя
		// строка не должна уводить её к концу — там читают дамп.
		if !m.debugOpen {
			m.view.GotoBottom()
		}
		return m, m.feed(line)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// feed исполняет ход в своей горутине: вызовы моделей идут секундами, и
// держать на них цикл отрисовки значит морозить экран.
func (m model) feed(line string) tea.Cmd {
	s := m.session
	return func() tea.Msg {
		if s == nil {
			return doneMsg{}
		}
		// События хода уходят в канал сами: приёмник сессии подменён в Run.
		return doneMsg{quit: s.Feed(line)}
	}
}

// quit — единая точка выхода. Закрывает done ровно один раз: если ход ещё
// работает в своей горутине и шлёт события через sink.Emit, закрытый канал
// done — это то, на чём Emit перестанет ждать место в events, которое уже
// никто не читает. Без этой развязки Ctrl-C посреди длинного хода (>64
// событий в буфере) навечно вешает горутину feed.
func (m model) quit() (tea.Model, tea.Cmd) {
	if !m.quitting {
		m.quitting = true
		close(m.done)
	}
	return m, tea.Quit
}

func (m *model) refresh() {
	m.resize()
	if m.debugOpen {
		// Дамп обмена — самые длинные строки в игре: промпт уезжает за край
		// без переноса так же, как проза.
		// Заголовок обязателен: без него дамп обмена неотличим от испорченного
		// транскрипта — и был принят именно за него.
		m.view.SetContent(debugTitle + "\n\n" +
			strings.Join(wrap(m.debugText(), m.width), "\n"))
		return
	}
	m.view.SetContent(m.transcript.Render(m.width))
}

// resize отдаёт транскрипту то, что осталось от экрана после обвязки.
//
// Раньше высота была константой «минус четыре» — заголовок, ввод, подвал и
// перевод строки. Приглашение уже нарушало этот счёт молча, а панель вариантов
// нарушила бы его на четыре строки: транскрипт уезжал бы под ввод. Считается
// по тому, что реально будет напечатано.
func (m *model) resize() {
	const chrome = 3 // заголовок, строка ввода, подвал
	left := m.height - chrome - lines(m.promptView()) - lines(m.optionsPane())
	if left < 1 {
		// Экран может быть меньше обвязки — на этом ломается любая
		// арифметика раскладки. Одна строка транскрипта хуже отрицательной.
		left = 1
	}
	m.view.Height = left
}

// promptView — приглашение у строки ввода либо пусто.
func (m model) promptView() string {
	if m.prompt == "" {
		return ""
	}
	return lipgloss.NewStyle().Bold(true).Render(m.prompt)
}

// optionsPane — панель вариантов, если ей место: под дампом обмена её нет.
func (m model) optionsPane() string {
	if m.debugOpen {
		return ""
	}
	return m.optionsView()
}

// lines — сколько строк занимает блок. Пустой блок не занимает ни одной: он и
// не печатается.
func lines(block string) int {
	if block == "" {
		return 0
	}
	return strings.Count(block, "\n") + 1
}

func (m model) debugText() string {
	if m.opts.Debug == nil {
		if m.opts.NoDebugReason != "" {
			return m.opts.NoDebugReason
		}
		return "отладка выключена: запустите с -debug-llm"
	}
	text := m.opts.Debug.Text()
	// Кольцо может существовать (нужно как приёмник внештатных сообщений —
	// алерта леджера, «Мастер не ответил») и быть пустым, пока их не было и
	// -debug-llm не задан: пустой экран по Tab тогда читается как поломка
	// панели, а не как «дампа не просили». NoDebugReason в этом случае
	// объясняет пустоту вместо того, чтобы молчать.
	if text == "" && m.opts.NoDebugReason != "" {
		return m.opts.NoDebugReason
	}
	if n := m.opts.Debug.Dropped(); n > 0 {
		text = fmt.Sprintf("(сброшено записей: %d)\n\n%s", n, text)
	}
	return text
}

// promptText готовит EventPrompt для показа у строки ввода: хвост "> " —
// это построчный курсор cli, полноэкранному режиму он не нужен, а трогать
// сам текст события в cli нельзя — построчный вывод обязан остаться
// побайтово прежним.
func promptText(text string) string {
	text = strings.TrimSuffix(text, "> ")
	return strings.TrimRight(text, "\n")
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	head := m.opts.Title
	if m.opts.Status != nil {
		head += " · " + m.opts.Status()
	}
	foot := "↑↓ варианты · ^P/^N история · PgUp/PgDn прокрутка · Tab отладка · ^C выход"
	switch {
	case m.ended:
		// Развязка уже дочитана до этой строки — подвал говорит, что дальше
		// делать, а не врёт «ход идёт».
		foot = "игра окончена — любая клавиша закрывает окно"
	case m.busy:
		foot = "ход идёт…  " + foot
	}
	frame := lipgloss.NewStyle().Faint(true)
	input := m.input.View()
	if prompt := m.promptView(); prompt != "" {
		// Приглашение — подсказка у строки ввода, а не строка транскрипта:
		// см. §2.1 спека.
		input = prompt + "\n" + input
	}
	parts := []string{frame.Render(head), m.view.View()}
	// Панель вариантов прячется, пока открыт дамп обмена: там читают промпт, а
	// не выбирают ход, и место экрана дороже.
	if opts := m.optionsPane(); opts != "" {
		parts = append(parts, opts)
	}
	return strings.Join(append(parts, input, frame.Render(foot)), "\n")
}
