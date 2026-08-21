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
)

// Options — что полноэкранному режиму нужно знать о запуске. Status отдаёт
// строку шапки (расход, ход): собирать её здесь значило бы тянуть в интерфейс
// слой моделей, который к экрану отношения не имеет.
type Options struct {
	Title  string
	Status func() string
	Debug  *Ring
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
	busy      bool
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
		m.view.Height = msg.Height - 4
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
		m.transcript.Append(msg.event)
		m.refresh()
		m.view.GotoBottom()
		return m, waitEvent(m.events)

	case doneMsg:
		m.busy = false
		if msg.quit {
			return m.quit()
		}
		return m, nil

	case tea.KeyMsg:
		return m.key(msg)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		// Выходим сразу, даже если ход ещё идёт (busy) — так было в брифе,
		// и это сохранено. Не сохранено только слепое доверие к тому, что
		// после tea.Quit больше некому дочитать events: за это отвечает
		// quit(), закрывая done.
		return m.quit()
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
		if line, ok := m.history.Prev(); ok {
			m.input.SetValue(line)
			m.input.CursorEnd()
		}
		return m, nil
	case tea.KeyDown:
		line, _ := m.history.Next()
		m.input.SetValue(line)
		m.input.CursorEnd()
		return m, nil
	case tea.KeyEnter:
		if m.busy {
			return m, nil
		}
		line := strings.TrimSpace(m.input.Value())
		if line == "" {
			return m, nil
		}
		m.history.Add(line)
		m.input.SetValue("")
		m.busy = true
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
	if m.debugOpen {
		m.view.SetContent(m.debugText())
		return
	}
	m.view.SetContent(m.transcript.Render(m.width))
}

func (m model) debugText() string {
	if m.opts.Debug == nil {
		return "отладка выключена: запустите с -debug-llm"
	}
	text := m.opts.Debug.Text()
	if n := m.opts.Debug.Dropped(); n > 0 {
		text = fmt.Sprintf("(сброшено записей: %d)\n\n%s", n, text)
	}
	return text
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	head := m.opts.Title
	if m.opts.Status != nil {
		head += " · " + m.opts.Status()
	}
	foot := "↑↓ история · PgUp/PgDn прокрутка · Tab отладка · ^C выход"
	if m.busy {
		foot = "ход идёт…  " + foot
	}
	frame := lipgloss.NewStyle().Faint(true)
	return strings.Join([]string{
		frame.Render(head),
		m.view.View(),
		m.input.View(),
		frame.Render(foot),
	}, "\n")
}
