package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kliuchnikovv/dnd/cli"
)

// Ввод уходит в игру по Enter и попадает в историю: без этого стрелка вверх
// пуста, а ради неё всё и затевалось.
func TestEnterSendsInputAndRemembersIt(t *testing.T) {
	m := newModel(nil, Options{})
	// Стартовая сцена «допечатана» — иначе Enter заблокирован тем же busy,
	// что и ход (см. TestEnterIsBlockedUntilStartFinishes ниже).
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	m.input.SetValue("осмотреться")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.input.Value() != "" {
		t.Errorf("строка ввода не очищена: %q", m.input.Value())
	}
	if got, ok := m.history.Prev(); !ok || got != "осмотреться" {
		t.Errorf("ввод не попал в историю: %q (%v)", got, ok)
	}
}

// Enter, нажатый раньше, чем s.Start() допечатал стартовую сцену, не должен
// проходить: иначе s.Feed запустится параллельно с ещё живым s.Start, а
// cli.Session рассчитан только на строго последовательные вызовы.
func TestEnterIsBlockedUntilStartFinishes(t *testing.T) {
	m := newModel(nil, Options{})
	m.input.SetValue("рано")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.input.Value() != "рано" {
		t.Error("ввод принят до конца стартовой сцены")
	}
}

// startedMsg — единственный способ снять стартовую блокировку; без него
// стрелка вверх и Enter молчат вечно.
func TestStartedMsgUnblocksInput(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	if m.busy {
		t.Error("busy не снят после отметки о завершении старта")
	}
}

// Ширина по умолчанию ненулевая: до первого tea.WindowSizeMsg транскрипт
// всё равно должен рендериться в разумную колонку, а не в нулевую.
func TestDefaultWidthIsNotZero(t *testing.T) {
	m := newModel(nil, Options{})
	if m.width <= 0 {
		t.Errorf("ширина по умолчанию не задана: %d", m.width)
	}
}

// Пока ход идёт, ввод заблокирован: два хода одновременно ядро не переживёт.
func TestInputIsBlockedWhileTurnRuns(t *testing.T) {
	m := newModel(nil, Options{})
	m.busy = true
	m.input.SetValue("второй ход")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.input.Value() != "второй ход" {
		t.Error("ввод принят во время хода")
	}
}

// События игры попадают в транскрипт с автором.
func TestEventGoesToTranscript(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(eventMsg{cli.Event{
		Kind: cli.EventSpeech, Speaker: "Берн", Text: "— Добрый день.\n"}})
	m = next.(model)

	if !strings.Contains(m.transcript.Render(60), "Берн") {
		t.Error("событие не доехало до транскрипта")
	}
}

// Tab переключает панель отладки и обратно.
func TestTabTogglesDebugPane(t *testing.T) {
	m := newModel(nil, Options{Debug: NewRing(4)})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !next.(model).debugOpen {
		t.Fatal("Tab не открыл панель отладки")
	}
	next, _ = next.(model).Update(tea.KeyMsg{Type: tea.KeyTab})
	if next.(model).debugOpen {
		t.Error("Tab не закрыл панель отладки")
	}
}

// КРИТИЧНО: развязка обязана быть видна. doneMsg{quit:true} раньше сразу
// вёл в tea.Quit — программа гасла, View() в состоянии выхода отдаёт пустую
// строку, и игрок не видел ни «Обвинение верно», ни клауз саммации, ни
// текста висяка. Постановление контроллера: конец игры не выходит сразу, а
// ждёт любую клавишу, показав финальный кадр.
func TestGameEndDoesNotQuitImmediately(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	next, _ = m.Update(doneMsg{quit: true})
	m = next.(model)

	if m.quitting {
		t.Fatal("конец игры завершил программу немедленно, не показав развязку")
	}
	if !m.ended {
		t.Fatal("состояние конца игры не отмечено")
	}
	if m.View() == "" {
		t.Error("экран пуст сразу после развязки — её нечем прочитать")
	}
}

// События, которые ещё летели по каналу в момент doneMsg (клаузы саммации,
// последствия), обязаны дойти до транскрипта, а не потеряться. Раньше
// tea.Quit убивал цикл программы немедленно и вместе с ним — недочитанные
// события; теперь цепочка waitEvent продолжает работать и после doneMsg.
// Несущий механизм правки — не специальный код в doneMsg, а то, что цепочка
// waitEvent, запущенная предыдущим eventMsg, ПРОДОЛЖАЕТ работать и после
// конца игры: doneMsg{quit:true} теперь не возвращает tea.Quit, а
// Update(eventMsg{...}) в состоянии ended переиздаёт подписку на канал, как
// и в обычном ходе. Проверяем обе половины напрямую, а не только конечный
// эффект (текст в транскрипте) — старый код печатал текст в транскрипт при
// любом Update(eventMsg{...}) независимо от quitting, так что одной проверки
// текста для отличия от tea.Quit-варианта недостаточно.
func TestGameEndKeepsEventChainAliveAndDoesNotQuitImmediately(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	next, doneCmd := m.Update(doneMsg{quit: true})
	m = next.(model)
	if doneCmd != nil {
		t.Fatal("doneMsg{quit:true} выдал команду — раньше это был tea.Quit, гасивший программу немедленно")
	}

	next, eventCmd := m.Update(eventMsg{cli.Event{
		Kind: cli.EventSystem, Text: "Обвинение верно. Попыток: 1.\n"}})
	m = next.(model)
	if eventCmd == nil {
		t.Fatal("после конца игры Update(eventMsg) не переиздал подписку на канал — цепочка waitEvent прервана")
	}
	if !strings.Contains(m.transcript.Render(60), "Обвинение верно") {
		t.Error("событие, дошедшее после развязки, потеряно")
	}
}

// После развязки любая клавиша (не только Ctrl-C) закрывает окно — это и
// есть «дочитал, нажал что угодно, вышел» из постановления.
func TestAnyKeyAfterGameEndQuits(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)
	next, _ = m.Update(doneMsg{quit: true})
	m = next.(model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if !m.quitting {
		t.Error("клавиша после развязки не завершила программу")
	}
	if cmd == nil {
		t.Error("после развязки не выдана команда tea.Quit")
	}
}

// Ctrl-C выходит немедленно и после развязки — обычный конец игры не должен
// требовать подтверждения нажатием именно той клавиши, которую и так нажали
// бы, чтобы прервать зависшую игру.
func TestCtrlCQuitsEvenAfterGameEnd(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)
	next, _ = m.Update(doneMsg{quit: true})
	m = next.(model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)
	if !m.quitting {
		t.Error("Ctrl-C после развязки не завершил программу")
	}
}

// EventPrompt — приглашение слота обвинения либо уточняющий вопрос — не
// строка транскрипта: спек §2.1 требует показывать его подсказкой у строки
// ввода, а не построчным блоком под подписью «Мастер».
func TestPromptEventShownNearInputNotInTranscript(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	next, _ = m.Update(eventMsg{cli.Event{
		Kind: cli.EventPrompt, Text: "who: a | b\n> "}})
	m = next.(model)

	if strings.Contains(m.transcript.Render(60), "who:") {
		t.Error("приглашение утекло в транскрипт")
	}
	view := m.View()
	if !strings.Contains(view, "who: a | b") {
		t.Error("приглашение не показано у строки ввода")
	}
	if strings.Contains(view, "b\n> ") {
		t.Error("хвост построчного курсора \"> \" показан в полноэкранном виде")
	}
}

// Пустой ввод — валидный ответ на слот обвинения (и на уточняющий вопрос) в
// построчном режиме: sc.Scan() отдаёт пустые строки как есть. cli не отдаёт
// публичного признака «жду ответ», поэтому модель равняется на то, что уже
// доступно: последнее событие было EventPrompt.
func TestEmptyEnterAllowedRightAfterPrompt(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)
	next, _ = m.Update(eventMsg{cli.Event{
		Kind: cli.EventPrompt, Text: "who: a | b\n> "}})
	m = next.(model)

	m.input.SetValue("")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if !m.busy {
		t.Error("пустой ответ на приглашение не запустил ход")
	}
	if cmd == nil {
		t.Error("пустой ответ на приглашение не выдал команду")
	}
}

// Без предшествующего приглашения пустой Enter остаётся заблокирован — это
// регрессия, которую легко внести, обобщая правило выше слишком широко.
func TestEmptyEnterStillBlockedWithoutPrompt(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	m.input.SetValue("")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.busy {
		t.Error("пустой ввод без приглашения запустил ход")
	}
}

// Пока открыта панель отладки, событие транскрипта не должно прокручивать
// её к концу: там читают дамп во время хода, и прыжок в конец на каждом
// событии делает чтение невозможным.
func TestGotoBottomDoesNotJumpWhileDebugOpen(t *testing.T) {
	m := newModel(nil, Options{Debug: NewRing(100)})
	next, _ := m.Update(startedMsg{})
	m = next.(model)
	for i := 0; i < 50; i++ {
		fmt.Fprintf(m.opts.Debug, "строка %d\n", i)
	}
	m.view.Height = 5

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if !m.debugOpen {
		t.Fatal("Tab не открыл панель отладки")
	}
	m.view.SetYOffset(0)

	next, _ = m.Update(eventMsg{cli.Event{Kind: cli.EventSystem, Text: "что-то произошло\n"}})
	m = next.(model)

	if m.view.YOffset != 0 {
		t.Errorf("прокрутка прыгнула во время открытой отладки: YOffset=%d", m.view.YOffset)
	}
}

// Без кольца панель отдаёт общий текст «запустите с -debug-llm». Когда
// причина другая (флаг дан, но -nl нет), Options.NoDebugReason должен
// перекрыть его — иначе панель отвечает тому, кто флаг и указал, будто он
// забыл это сделать.
func TestNoDebugReasonOverridesGenericMessage(t *testing.T) {
	m := newModel(nil, Options{NoDebugReason: "-debug-llm без -nl ничего не даёт"})
	m.debugOpen = true
	if got := m.debugText(); got != "-debug-llm без -nl ничего не даёт" {
		t.Errorf("панель показала %q, ожидалась причина из Options", got)
	}
}

// Кольцо заведено (в fullscreen+-nl оно всегда есть — приёмник алерта
// леджера и «Мастер не ответил»), но пусто, потому что -debug-llm не
// указан: панель обязана объяснить пустоту через NoDebugReason, а не
// показать голый пустой экран, который читается как поломка.
func TestNoDebugReasonShownWhenRingExistsButEmpty(t *testing.T) {
	m := newModel(nil, Options{
		Debug:         NewRing(10),
		NoDebugReason: "дамп обмена выключен: здесь только внештатные сообщения, для полного дампа запустите с -debug-llm",
	})
	m.debugOpen = true
	if got := m.debugText(); got != m.opts.NoDebugReason {
		t.Errorf("пустое кольцо показало %q, ожидалась причина из Options", got)
	}
}

// Как только в кольце появился хоть один обмен или внештатное сообщение,
// причина перестаёт быть релевантной: показывать надо содержимое, а не
// прятать его за NoDebugReason.
func TestNoDebugReasonHiddenWhenRingHasContent(t *testing.T) {
	r := NewRing(10)
	fmt.Fprint(r, "расход 100 мкд превысил прогноз 50 вдвое\n")
	m := newModel(nil, Options{Debug: r, NoDebugReason: "причина, которая не должна перекрыть содержимое"})
	m.debugOpen = true
	if got := m.debugText(); !strings.Contains(got, "расход 100 мкд") {
		t.Errorf("причина перекрыла непустое кольцо: %q", got)
	}
}

// Приглашение висит над строкой ввода, пока игра ждёт ответа. Ответил — оно
// обязано исчезнуть: иначе игрок читает уже отвеченный вопрос как заданный
// снова и отвечает второй раз.
func TestAnsweredPromptDisappears(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(eventMsg{cli.Event{Kind: cli.EventPrompt, Text: "чем именно?\n"}})
	m = next.(model)
	if m.prompt == "" {
		t.Fatal("приглашение не показано")
	}

	m.busy = false
	m.input.SetValue("рукой")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := next.(model).prompt; got != "" {
		t.Errorf("после ответа приглашение всё ещё висит: %q", got)
	}
}

// У панели отладки обязан быть заголовок: без него дамп обмена неотличим от
// испорченного транскрипта — и был принят именно за него.
func TestDebugPaneIsLabelled(t *testing.T) {
	ring := NewRing(4)
	ring.Write([]byte("--- llm actor -> провайдер ---\nввод: ...\n"))
	m := newModel(nil, Options{Debug: ring})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)

	if !m.debugOpen {
		t.Fatal("панель не открылась")
	}
	if got := m.view.View(); !strings.Contains(got, "обмен с моделью") {
		t.Errorf("панель без заголовка:\n%s", got)
	}
}

// Enter обязан положить свою строку в транскрипт ДО того, как ход что-то
// ответит: иначе игрок не видит, на что отвечают, и разговор читается как
// монолог мира.
func TestEnterEchoesInputIntoTranscript(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	m.input.SetValue("осмотреть бочки")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if !strings.Contains(m.transcript.Render(60), "осмотреть бочки") {
		t.Errorf("ввод не попал в транскрипт:\n%s", m.transcript.Render(60))
	}
}

// Пустая строка — валидный ответ на вопрос игры (слот обвинения), но эхом
// она печатается пустым блоком под подписью: подпись без слов.
func TestEmptyAnswerIsNotEchoed(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)
	m.lastKind = cli.EventPrompt

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if strings.Contains(m.transcript.Render(60), cli.PlayerName) {
		t.Errorf("пустой ответ отмечен подписью:\n%s", m.transcript.Render(60))
	}
}

// Колесо мыши обязано крутить транскрипт, а не скроллбек терминала. Без
// перехвата игрок, потянувшийся к истории разговора, видит историю шелла: свои
// же команды и метрики прошлых прогонов вместо разговора с Берном.
func TestWheelScrollsTheTranscript(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m = next.(model)
	for i := 0; i < 60; i++ {
		next, _ = m.Update(eventMsg{cli.Event{Kind: cli.EventProse,
			Text: fmt.Sprintf("строка %d\n", i)}})
		m = next.(model)
	}
	if m.view.AtTop() {
		t.Fatal("транскрипт не набрал высоты — тест ничего не проверяет")
	}

	next, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	up := next.(model)
	if up.view.YOffset >= m.view.YOffset {
		t.Errorf("колесо вверх не прокрутило транскрипт: смещение %d при %d",
			up.view.YOffset, m.view.YOffset)
	}

	next, _ = up.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	down := next.(model)
	if down.view.YOffset <= up.view.YOffset {
		t.Errorf("колесо вниз не вернуло транскрипт: смещение %d при %d",
			down.view.YOffset, up.view.YOffset)
	}
}

// Колесо не должно попадать в строку ввода: там оно ничего не значит, а
// default-ветка Update отдаёт неразобранное именно ей.
func TestWheelDoesNotReachTheInput(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)
	m.input.SetValue("предъявить предписание Берну")

	next, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if got := next.(model).input.Value(); got != "предъявить предписание Берну" {
		t.Errorf("колесо изменило ввод: %q", got)
	}
}
