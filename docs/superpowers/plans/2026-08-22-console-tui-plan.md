# Консольный чат: полноэкранный режим. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** дать игре полноэкранный консольный чат: правка строки, история ввода, прокрутка транскрипта, метки говорящего и панель отладки.

**Architecture:** цикл инвертируется — `Session.Feed(line)` исполняет один ввод, `Run()` остаётся построчным драйвером для скриптов и тестов, новый пакет `tui/` становится вторым драйвером того же `Feed`. Вывод сессии превращается из байтов в события `Event{Kind, Speaker, Text}`; `TextSink` печатает их побайтово так же, как сегодня, а TUI по `Kind` рисует метки и разделители.

**Tech Stack:** Go 1.26, `charmbracelet/bubbletea`, `charmbracelet/bubbles`, `charmbracelet/lipgloss`.

**Spec:** `docs/superpowers/specs/2026-08-22-console-tui-design.md`

## Global Constraints

- Домен не трогается: `core`, `rules`, `store`, `cases`, `dice` не получают ни одной правки и не узнают о `tui`. Это проверяет `e2e/architecture_test.go` — он обязан остаться зелёным.
- `cli` НЕ импортирует `tui`. Зависимость только в одну сторону: `tui` → `cli`.
- Вывод построчного режима остаётся **побайтово** прежним. Существующие тесты `cli` — спецификация формата; менять их ожидания нельзя.
- Воспроизводимость по паре `(seed, script)` не трогается: не-терминальный путь идёт через `Run()`, как раньше.
- `go test ./...` и `go vet ./...` зелёные после каждой задачи. Красный коммит недопустим.
- Комментарии по-русски, объясняют ПОЧЕМУ, а не ЧТО, — как в остальном репозитории.

---

### Task 1: События вывода и `TextSink`

**Files:**
- Create: `cli/event.go`
- Modify: `cli/repl.go` (поле `Out` → `sink`, все `fmt.Fprint*(s.Out, …)`), `cli/interpret.go`, `cli/voice.go`
- Test: `cli/event_test.go`

**Interfaces:**
- Produces: `cli.EventKind`, константы `cli.EventScene|EventProse|EventSpeech|EventSystem|EventRefusal|EventPrompt|EventNote`, `cli.Event{Kind, Speaker, Text string}`, `cli.Sink` с методом `Emit(Event)`, `cli.TextSink{W io.Writer}`, `(*Session).WithSink(Sink) *Session`, внутренние `(*Session).emit(EventKind, string, ...any)` и `(*Session).emitText(EventKind, string)`.

- [ ] **Step 1: Написать падающий тест**

Создать `cli/event_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

// TextSink печатает событие дословно: построчный вывод обязан остаться
// побайтово тем же, иначе скрипты и тесты начнут расходиться с игрой.
func TestTextSinkPrintsTextVerbatim(t *testing.T) {
	var out strings.Builder
	sink := TextSink{W: &out}
	sink.Emit(Event{Kind: EventSystem, Text: "== Пристань ==\n"})
	sink.Emit(Event{Kind: EventSpeech, Speaker: "Берн", Text: "— Добрый день.\n"})

	want := "== Пристань ==\n— Добрый день.\n"
	if got := out.String(); got != want {
		t.Errorf("напечатано %q, ждали %q", got, want)
	}
}

// Сессия пишет в приёмник, а не в io.Writer: без этого «кто говорит» можно
// узнать только разбором собственного напечатанного текста.
func TestSessionEmitsToSink(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	s := NewSession(g, strings.NewReader("quit\n"), &strings.Builder{}).WithSink(rec)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if len(rec.events) == 0 {
		t.Fatal("сессия не отдала ни одного события")
	}
	if rec.events[0].Kind != EventScene {
		t.Errorf("первое событие %q, ждали сцену", rec.events[0].Kind)
	}
}

type recordSink struct{ events []Event }

func (r *recordSink) Emit(e Event) { r.events = append(r.events, e) }
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./cli/ -run 'TextSink|SessionEmits'`
Expected: FAIL — `undefined: TextSink`, `undefined: Event`.

- [ ] **Step 3: Завести типы событий**

Создать `cli/event.go`:

```go
package cli

import (
	"fmt"
	"io"
)

// EventKind — что это за строка вывода. Без вида и автора метку говорящего
// пришлось бы восстанавливать разбором напечатанного текста, то есть парсить
// собственный вывод.
type EventKind string

const (
	EventScene   EventKind = "scene"   // описание места
	EventProse   EventKind = "prose"   // проза Мастера об исходе хода
	EventSpeech  EventKind = "speech"  // прямая речь: NPC или игрок
	EventSystem  EventKind = "system"  // справка, факты, часы, состояние
	EventRefusal EventKind = "refusal" // «нельзя: …»
	EventPrompt  EventKind = "prompt"  // игра спросила: слот обвинения, уточнение
	EventNote    EventKind = "note"    // поломка надстройки: «Мастер промолчал»
)

// Event — единица вывода. Text уже отрендерен: приёмник его не собирает и не
// разбирает, а показывает. Так построчный режим остаётся побайтово прежним.
type Event struct {
	Kind    EventKind
	Speaker string // «Берн, стражник» либо «Вы»; только у EventSpeech
	Text    string
}

// Sink — куда уходит вывод сессии. Интерфейс нужен, чтобы полноэкранный режим
// был вторым приёмником, а не вторым форматом вывода.
type Sink interface{ Emit(Event) }

// TextSink печатает событие как есть. Это сегодняшнее поведение игры целиком:
// вид и автор ему не нужны, потому что текст уже собран.
type TextSink struct{ W io.Writer }

func (t TextSink) Emit(e Event) { fmt.Fprint(t.W, e.Text) }
```

- [ ] **Step 4: Перевести сессию на приёмник**

В `cli/repl.go` в структуру `Session` добавить поле и метод:

```go
	// sink — куда уходит вывод. Сессия печатает только через него: иначе
	// полноэкранный режим пришлось бы делать вторым форматом вывода, а не
	// вторым приёмником.
	sink Sink
```

В `NewSession` завернуть writer:

```go
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
```

- [ ] **Step 5: Перевести все места печати**

В `cli/repl.go`, `cli/interpret.go`, `cli/voice.go` заменить каждый `fmt.Fprint*(s.Out, …)` на `s.emit(...)`/`s.emitText(...)` с видом по таблице:

| Было | Стало |
|---|---|
| `fmt.Fprint(s.Out, s.r.Scene(s.Game))` | `s.emitText(EventScene, s.r.Scene(s.Game))` |
| `fmt.Fprint(s.Out, r.Help()/Survey/Facts/State/Clocks)` | `s.emitText(EventSystem, …)` |
| `fmt.Fprint(s.Out, r.Turn(…))` | `s.emitText(EventProse, …)` |
| `fmt.Fprintf(s.Out, "нельзя: %v\n", …)` | `s.emit(EventRefusal, "нельзя: %v\n", …)` |
| `fmt.Fprintf(s.Out, "%s: %s\n> ", …)` (обвинение) | `s.emit(EventPrompt, "%s: %s\n> ", …)` |
| `fmt.Fprintf(s.Out, "%s\n", question)` (уточнение) | `s.emit(EventPrompt, "%s\n", question)` |
| `fmt.Fprintf(s.Out, "(персонаж промолчал: %v)\n", err)` | `s.emit(EventNote, "(персонаж промолчал: %v)\n", err)` |
| `fmt.Fprintf(s.Out, "(%s)\n", text)` в `noteOnce` | `s.emit(EventNote, "(%s)\n", text)` |

Остальные места `repl.go` поимённо, чтобы не осталось ни одного `s.Out`:

| Строка | Стало |
|---|---|
| `"\n%s\n", cold` (висяк в `ended`) | `s.emit(EventSystem, "\n%s\n", cold)` |
| `"длинный отдых продвинет часы: %v\n", clocks` | `s.emit(EventSystem, …)` |
| `"%s: %s\n", e.Name, line` (подсказка напарника) | `s.sink.Emit(Event{Kind: EventSpeech, Speaker: e.Name, Text: fmt.Sprintf("%s: %s\n", e.Name, line)})` |
| `s.r.Scene(s.Game)` после перемещения | `s.emitText(EventScene, s.r.Scene(s.Game))` |
| `"к кому ты обращаешься? здесь %s\n", …` | `s.emit(EventPrompt, …)` |
| `Spoken(said)` (речь игрока) | пока `s.emit(EventSystem, "%s\n", Spoken(said))`, автор появится в задаче 2 |
| строки внутри `accuse()` | не трогать: `accuse()` целиком переезжает в задаче 3 |

После правки проверить, что `s.Out` не осталось нигде, кроме `NewSession`:

```bash
grep -n "s.Out" cli/*.go | grep -v _test
```

Ожидается: ни одной строки.

- [ ] **Step 6: Прогнать всю сюиту**

Run: `go test ./... && go vet ./...`
Expected: PASS. Существующие тесты `cli` сравнивают напечатанное — если хоть один упал, формат изменён, и это ошибка перевода, а не тест.

- [ ] **Step 7: Коммит**

```bash
git add cli/event.go cli/event_test.go cli/repl.go cli/interpret.go cli/voice.go
git commit -m "feat(cli): вывод становится событиями, а не байтами"
```

---

### Task 2: Метка говорящего у прямой речи

**Files:**
- Modify: `cli/voice.go` (`speak`), `cli/repl.go` (`act`, где печатается речь игрока)
- Test: `cli/event_test.go`

**Interfaces:**
- Consumes: `cli.Event`, `cli.Sink`, `(*Session).sink` из Task 1.
- Produces: события `EventSpeech` с заполненным `Speaker`; константа `cli.PlayerName = "Вы"`.

- [ ] **Step 1: Написать падающий тест**

Дописать в `cli/event_test.go`:

```go
// Метка говорящего берётся из данных, а не из тире в начале строки: разбирать
// собственный вывод, чтобы понять, кто сказал, — та же ошибка, что читать
// канон из прозы.
func TestSpeechCarriesSpeaker(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	fv := &fakeVoicer{line: "Мокро сегодня."}
	s := NewSession(g, strings.NewReader("talk_to toke\nquit\n"), &strings.Builder{}).
		WithSink(rec).WithVoicer(fv)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}

	var speech []Event
	for _, e := range rec.events {
		if e.Kind == EventSpeech {
			speech = append(speech, e)
		}
	}
	if len(speech) != 1 {
		t.Fatalf("событий речи %d, ждали одно", len(speech))
	}
	if !strings.Contains(speech[0].Speaker, "Токе") {
		t.Errorf("говорящий %q", speech[0].Speaker)
	}
	if !strings.Contains(speech[0].Text, "Мокро сегодня.") {
		t.Errorf("реплика %q", speech[0].Text)
	}
}

// Речь игрока — тоже речь, и у неё тоже есть автор.
func TestPlayerSpeechIsMarkedAsPlayers(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	fi := &fakeInterp{intent: &core.Intent{Verb: "say",
		Args: core.Args{Target: "e_toke", Text: "здравствуйте"}}}
	s := NewSession(g, strings.NewReader("здравствуйте\nquit\n"), &strings.Builder{}).
		WithSink(rec).WithInterpreter(fi)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	for _, e := range rec.events {
		if e.Kind == EventSpeech && e.Speaker == PlayerName {
			return
		}
	}
	t.Errorf("речь игрока не помечена автором: %+v", rec.events)
}
```

`fakeVoicer` уже объявлен в `cli/repl_test.go:224` — он в том же пакете, объявлять заново не нужно.

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./cli/ -run 'Speech'`
Expected: FAIL — `undefined: PlayerName`, событий речи ноль.

- [ ] **Step 3: Реализовать**

В `cli/voice.go`:

```go
// PlayerName — как подписана речь игрока. Одно место на всю игру: подпись
// участвует в раскладке полноэкранного режима, и расхождение подписей там
// видно сразу.
const PlayerName = "Вы"
```

В `speak` вместо печати:

```go
	if line != "" {
		s.sink.Emit(Event{Kind: EventSpeech,
			Speaker: s.speakerName(in.Args.Target), Text: Spoken(line) + "\n"})
	}
```

Рядом завести:

```go
// speakerName — имя сущности для подписи реплики. Пустое имя означает, что
// говорит не человек, и подпись тогда не нужна.
func (s *Session) speakerName(id store.EntityID) string {
	return s.Game.DB.Entities[id].Name
}
```

В `cli/repl.go` в `act`, где печатается сказанное игроком вслух:

```go
	if said := spokenAloud(in); said != "" && !res.Refused {
		s.sink.Emit(Event{Kind: EventSpeech, Speaker: PlayerName, Text: Spoken(said) + "\n"})
	} else {
		s.emitText(EventProse, s.r.Turn(s.Game, in, res))
	}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Коммит**

```bash
git add cli/voice.go cli/repl.go cli/event_test.go
git commit -m "feat(cli): у прямой речи есть автор"
```

---

### Task 3: Обвинение как автомат состояний

**Files:**
- Create: `cli/accuse.go` (перенос `accuse()` из `cli/repl.go`)
- Modify: `cli/repl.go` (удалить `accuse()`, добавить состояние и ветку в цикл)
- Test: `cli/accuse_test.go`

**Interfaces:**
- Consumes: `(*Session).emit`, `EventPrompt`, `EventSystem` из Task 1.
- Produces: `(*Session).startAccusation()`, `(*Session).feedAccusation(line string)`, `(*Session).awaitingAccusation() bool`, поле `accusing *accusation4`.

- [ ] **Step 1: Написать падающий тест**

Создать `cli/accuse_test.go`:

```go
package cli

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// Обвинение читало четыре строки вложенным циклом. Полноэкранный режим отдаёт
// ввод по одной строке и заблокироваться не может, поэтому набор слотов —
// состояние сессии, а не вложенный цикл.
func TestAccusationFillsSlotsOneFeedAtATime(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithSink(rec)

	s.startAccusation()
	if !s.awaitingAccusation() {
		t.Fatal("после начала обвинения сессия не ждёт токен")
	}
	for i, tok := range []string{"toke", "cord", "night", "audit"} {
		if !s.awaitingAccusation() {
			t.Fatalf("на слоте %d сессия перестала ждать ввод", i)
		}
		s.feedAccusation(tok)
	}
	if s.awaitingAccusation() {
		t.Error("после четырёх токенов обвинение всё ещё ждёт ввод")
	}
	if !s.Game.Solved() {
		t.Error("верное обвинение не закрыло дело")
	}
}

// Пустой слот прекращает набор, а не вешает сессию в ожидании ввода, которого
// игрок дать не может.
func TestEmptySlotEndsAccusation(t *testing.T) {
	// Игра без токенов вовсе: фикстура minimal.json отдаёт слот who сразу, и
	// на ней пустой слот не воспроизвести.
	g := core.NewGame(core.Config{DB: store.NewDB()})
	rec := &recordSink{}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithSink(rec)
	s.startAccusation()
	if s.awaitingAccusation() {
		t.Error("сессия ждёт токен, которого взять негде")
	}
	var said string
	for _, e := range rec.events {
		said += e.Text
	}
	if !strings.Contains(said, "пуст") {
		t.Errorf("игроку не сказано, что слот пуст: %q", said)
	}
}
```

Токены `toke/cord/night/audit` — те же, что в существующем тесте `cli/repl_test.go:68`, который гонит `accuse` через настоящий цикл; фикстура `renderGame` грузит `cases/testdata/minimal.json`, где эти токены доступны сразу. Именно поэтому тест пустого слота строит игру с пустой базой: на `minimal.json` пустой слот не воспроизводится.

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./cli/ -run Accusation`
Expected: FAIL — `undefined: startAccusation`.

- [ ] **Step 3: Реализовать автомат**

Создать `cli/accuse.go`, перенеся туда тело старого `accuse()`:

```go
package cli

import (
	"strings"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// accusation4 — набор четырёх слотов обвинения. Состояние, а не вложенный
// цикл ввода: драйвер отдаёт по одной строке и ждать следующую внутри хода не
// может, а обвинение — единственный способ закончить дело.
type accusation4 struct {
	form accusation.Form
	idx  int
}

var accusationSlots = []struct {
	name string
	dst  func(*accusation.Form) *store.Token
}{
	{"who", func(f *accusation.Form) *store.Token { return &f.Who }},
	{"how", func(f *accusation.Form) *store.Token { return &f.How }},
	{"when", func(f *accusation.Form) *store.Token { return &f.When }},
	{"why", func(f *accusation.Form) *store.Token { return &f.Why }},
}

func (s *Session) awaitingAccusation() bool { return s.accusing != nil }

// startAccusation открывает набор и печатает приглашение первого слота.
func (s *Session) startAccusation() {
	s.accusing = &accusation4{}
	s.promptSlot()
}

// promptSlot печатает приглашение текущего слота либо закрывает набор, если
// слот пуст: подсказывать нечего, а ждать ввод, которого игрок дать не может,
// значит подвесить игру.
func (s *Session) promptSlot() {
	slot := accusationSlots[s.accusing.idx]
	avail := s.Game.AvailableTokens(slot.name)
	if len(avail) == 0 {
		s.emit(EventSystem, "слот %s пуст: нужных фактов ещё нет\n", slot.name)
		s.accusing = nil
		return
	}
	parts := make([]string, len(avail))
	for i, a := range avail {
		parts[i] = string(a)
	}
	s.emit(EventPrompt, "%s: %s\n> ", slot.name, strings.Join(parts, " | "))
}

// feedAccusation заполняет текущий слот и двигает набор дальше.
func (s *Session) feedAccusation(line string) {
	slot := accusationSlots[s.accusing.idx]
	*slot.dst(&s.accusing.form) = store.Token(strings.TrimSpace(line))
	s.accusing.idx++
	if s.accusing.idx < len(accusationSlots) {
		s.promptSlot()
		return
	}
	form := s.accusing.form
	s.accusing = nil
	s.resolveAccusation(form)
}

// resolveAccusation — исход обвинения. Ошибочная форма не сообщает, какой слот
// неверен: иначе слоты брутфорсятся по одному.
func (s *Session) resolveAccusation(form accusation.Form) {
	g := s.Game
	res := g.Accuse(form)
	switch {
	case res.Refused:
		s.emit(EventRefusal, "нельзя: %s\n", res.Refusal)
	case res.Correct:
		s.emit(EventSystem, "Обвинение верно. Попыток: %d.\n\n", res.Attempt)
		for _, clause := range res.Summation {
			s.emit(EventSpeech, "  — %s\n", clause)
		}
		if after := g.Aftermath(); after != "" {
			s.emit(EventSystem, "\n%s\n", after)
		}
	default:
		s.emit(EventSystem, "В обвинении есть ошибки. Попыток: %d.\n", res.Attempt)
	}
	for _, c := range res.Fired {
		s.emit(EventSystem, "  ⏱ %s\n", g.Flavour(c.FlavourKey))
	}
}
```

В `cli/repl.go`: удалить старый `accuse()`, добавить поле `accusing *accusation4` в `Session`, заменить `case CmdAccuse: s.accuse()` на `case CmdAccuse: s.startAccusation()`, а в теле цикла `Run` перед разбором команды поставить ветку:

```go
		if s.awaitingAccusation() {
			s.feedAccusation(line)
			if s.ended() {
				return nil
			}
			continue
		}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./... && go vet ./...`
Expected: PASS. Особенно важен существующий `cli/repl_test.go`, который гонит `accuse\ntoke\ncord\nnight\naudit\nquit` через настоящий цикл: он и есть проверка, что автомат ведёт себя как прежний вложенный цикл.

- [ ] **Step 5: Коммит**

```bash
git add cli/accuse.go cli/accuse_test.go cli/repl.go
git commit -m "refactor(cli): обвинение — автомат состояний, а не вложенный цикл ввода"
```

---

### Task 4: `Feed` и `Start` вместо тела цикла

**Files:**
- Modify: `cli/repl.go`
- Test: `cli/feed_test.go`

**Interfaces:**
- Consumes: `awaitingAccusation`, `feedAccusation` из Task 3.
- Produces: `(*Session).Start()`, `(*Session).Feed(line string) (done bool)`; `Run()` сохраняет прежнюю сигнатуру `() error`.

- [ ] **Step 1: Написать падающий тест**

Создать `cli/feed_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

// Полноэкранному режиму нужен ход по одной строке: циклом владеет он, а не
// сессия. Логика при этом та же самая, и оба драйвера обязаны давать
// одинаковый результат.
func TestFeedRunsOneTurn(t *testing.T) {
	g := renderGame(t)
	var out strings.Builder
	s := NewSession(g, strings.NewReader(""), &out)

	s.Start()
	if !strings.Contains(out.String(), "==") {
		t.Error("Start не напечатал стартовую сцену")
	}
	if done := s.Feed("look"); done {
		t.Error("осмотр закончил игру")
	}
	if done := s.Feed("quit"); !done {
		t.Error("quit не закончил игру")
	}
}

// Ход считается один раз на ввод: по нему считается потолок вызовов модели, и
// сбитый счёт означает сбитый лимит.
func TestFeedCountsTurns(t *testing.T) {
	g := renderGame(t)
	s := NewSession(g, strings.NewReader(""), &strings.Builder{})
	s.Start()
	s.Feed("look")
	s.Feed("look")
	if got := s.Turn(); got != 2 {
		t.Errorf("ходов %d, ждали 2", got)
	}
}

// Run — тот же Feed в цикле: построчный режим обязан остаться побайтово тем
// же, иначе скрипты и тесты разойдутся с игрой.
func TestRunAndFeedAgree(t *testing.T) {
	script := "look\nfacts\nquit\n"

	var viaRun strings.Builder
	if err := NewSession(renderGame(t), strings.NewReader(script), &viaRun).Run(); err != nil {
		t.Fatal(err)
	}

	var viaFeed strings.Builder
	s := NewSession(renderGame(t), strings.NewReader(""), &viaFeed)
	s.Start()
	for _, line := range []string{"look", "facts", "quit"} {
		if s.Feed(line) {
			break
		}
	}

	if viaRun.String() != viaFeed.String() {
		t.Errorf("драйверы разошлись:\n--- Run ---\n%s\n--- Feed ---\n%s",
			viaRun.String(), viaFeed.String())
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./cli/ -run 'Feed|RunAndFeed'`
Expected: FAIL — `undefined: (*Session).Start`.

- [ ] **Step 3: Реализовать**

В `cli/repl.go`:

```go
// Start печатает стартовую сцену. Отдельно от Run, потому что драйверов два:
// построчный читает stdin сам, полноэкранный владеет своим циклом.
func (s *Session) Start() {
	s.emitText(EventScene, s.r.Scene(s.Game))
}

// Feed исполняет ОДИН ввод и сообщает, пора ли заканчивать. Здесь живёт всё,
// что раньше было телом цикла Run: разбор, перевод свободного текста,
// исполнение и проверка развязки.
func (s *Session) Feed(line string) bool {
	s.turn++
	if s.awaitingAccusation() {
		s.feedAccusation(line)
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
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Коммит**

```bash
git add cli/repl.go cli/feed_test.go
git commit -m "refactor(cli): Feed исполняет один ввод, Run остаётся драйвером"
```

---

### Task 5: Кольцо истории ввода

**Files:**
- Create: `tui/history.go`, `tui/history_test.go`

**Interfaces:**
- Produces: `tui.History`, `(*History).Add(string)`, `(*History).Prev() (string, bool)`, `(*History).Next() (string, bool)`, `(*History).Reset()`.

- [ ] **Step 1: Написать падающий тест**

Создать `tui/history_test.go`:

```go
package tui

import "testing"

// Стрелка вверх поднимает последнее введённое, вниз возвращает обратно.
func TestHistoryWalksBackAndForth(t *testing.T) {
	var h History
	h.Add("осмотреться")
	h.Add("поговорить с Берном")

	if got, ok := h.Prev(); !ok || got != "поговорить с Берном" {
		t.Fatalf("первый шаг назад дал %q (%v)", got, ok)
	}
	if got, ok := h.Prev(); !ok || got != "осмотреться" {
		t.Fatalf("второй шаг назад дал %q (%v)", got, ok)
	}
	if _, ok := h.Prev(); ok {
		t.Error("шаг назад за начало истории удался")
	}
	if got, ok := h.Next(); !ok || got != "поговорить с Берном" {
		t.Errorf("шаг вперёд дал %q (%v)", got, ok)
	}
}

// Пустой ввод и повтор подряд историю не засоряют: иначе стрелка вверх
// перелистывает одно и то же.
func TestHistorySkipsEmptyAndRepeats(t *testing.T) {
	var h History
	h.Add("look")
	h.Add("look")
	h.Add("   ")
	h.Add("")

	if got, _ := h.Prev(); got != "look" {
		t.Errorf("последнее в истории %q", got)
	}
	if _, ok := h.Prev(); ok {
		t.Error("в истории больше одной записи — повторы и пустое не отсеяны")
	}
}

// После отправки история читается с конца: игрок ждёт последнюю фразу первым
// же нажатием, а не продолжения прошлой прогулки по списку.
func TestHistoryResetsPositionAfterAdd(t *testing.T) {
	var h History
	h.Add("first")
	h.Add("second")
	h.Prev()
	h.Prev()
	h.Add("third")
	if got, _ := h.Prev(); got != "third" {
		t.Errorf("после ввода стрелка вверх дала %q", got)
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./tui/`
Expected: FAIL — пакета нет.

- [ ] **Step 3: Реализовать**

Создать `tui/history.go`:

```go
// Package tui — полноэкранный консольный режим. Лежит НАД cli и необязателен:
// без него игра работает построчно, как раньше.
package tui

import "strings"

// History — кольцо введённых фраз. Своё, потому что у поля ввода библиотеки
// истории нет, а разговор на сорок ходов без повтора прошлой фразы не ведут.
type History struct {
	items []string
	// pos — куда смотрит игрок. Равен len(items), когда он не листает.
	pos int
}

// Add кладёт фразу и возвращает курсор в конец: после отправки стрелка вверх
// обязана дать последнее сказанное, а не продолжить прошлую прогулку.
func (h *History) Add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		h.pos = len(h.items)
		return
	}
	if n := len(h.items); n > 0 && h.items[n-1] == line {
		h.pos = len(h.items)
		return
	}
	h.items = append(h.items, line)
	h.pos = len(h.items)
}

// Prev — шаг назад по истории. false означает, что дальше некуда.
func (h *History) Prev() (string, bool) {
	if h.pos == 0 {
		return "", false
	}
	h.pos--
	return h.items[h.pos], true
}

// Next — шаг вперёд. false на выходе за конец: там пустая строка ввода.
func (h *History) Next() (string, bool) {
	if h.pos+1 >= len(h.items) {
		h.pos = len(h.items)
		return "", false
	}
	h.pos++
	return h.items[h.pos], true
}

// Reset возвращает курсор в конец, не трогая записи.
func (h *History) Reset() { h.pos = len(h.items) }
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./tui/ && go vet ./tui/`
Expected: PASS.

- [ ] **Step 5: Коммит**

```bash
git add tui/history.go tui/history_test.go
git commit -m "feat(tui): кольцо истории ввода"
```

---

### Task 6: Кольцевой буфер отладки

**Files:**
- Create: `tui/ring.go`, `tui/ring_test.go`

**Interfaces:**
- Produces: `tui.Ring`, `tui.NewRing(limit int) *Ring`, `(*Ring).Write([]byte) (int, error)` (реализует `io.Writer`), `(*Ring).Text() string`, `(*Ring).Dropped() int`.

- [ ] **Step 1: Написать падающий тест**

Создать `tui/ring_test.go`:

```go
package tui

import (
	"fmt"
	"strings"
	"testing"
)

// Дамп обмена за сессию — мегабайты. Буфер ограничен, но вытесненное
// считается: молча обрезанный лог выглядит как пропавшие вызовы.
func TestRingKeepsLastRecordsAndCountsDropped(t *testing.T) {
	r := NewRing(3)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(r, "запись %d\n", i)
	}

	text := r.Text()
	if strings.Contains(text, "запись 1") || strings.Contains(text, "запись 2") {
		t.Errorf("старое не вытеснено:\n%s", text)
	}
	if !strings.Contains(text, "запись 5") {
		t.Errorf("свежее потеряно:\n%s", text)
	}
	if got := r.Dropped(); got != 2 {
		t.Errorf("вытеснено %d записей, ждали 2", got)
	}
}

// Пустой буфер — не ошибка: с -debug-llm без вызовов модели показывать нечего.
func TestEmptyRingIsEmpty(t *testing.T) {
	r := NewRing(2)
	if r.Text() != "" || r.Dropped() != 0 {
		t.Errorf("пустой буфер не пуст: %q, вытеснено %d", r.Text(), r.Dropped())
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./tui/ -run Ring`
Expected: FAIL — `undefined: NewRing`.

- [ ] **Step 3: Реализовать**

Создать `tui/ring.go`:

```go
package tui

import (
	"strings"
	"sync"
)

// Ring — кольцевой буфер записей отладки. Шлюз пишет в io.Writer, поэтому
// панель отладки — это просто другой writer, а не другой формат дампа.
//
// Ограничение обязательно: одна сессия с -debug-llm даёт мегабайты, и держать
// их целиком ради прокрутки незачем. Вытесненное считается — молча обрезанный
// лог читается как пропавшие вызовы.
type Ring struct {
	mu      sync.Mutex
	limit   int
	items   []string
	dropped int
}

func NewRing(limit int) *Ring {
	if limit < 1 {
		limit = 1
	}
	return &Ring{limit: limit}
}

// Write принимает одну запись дампа. Шлюз пишет её одним вызовом, поэтому
// делить поток на строки не нужно.
func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, string(p))
	if len(r.items) > r.limit {
		r.dropped += len(r.items) - r.limit
		r.items = append([]string(nil), r.items[len(r.items)-r.limit:]...)
	}
	return len(p), nil
}

// Text — содержимое буфера от старого к новому.
func (r *Ring) Text() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.items, "")
}

// Dropped — сколько записей вытеснено.
func (r *Ring) Dropped() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./tui/ && go vet ./tui/`
Expected: PASS.

- [ ] **Step 5: Коммит**

```bash
git add tui/ring.go tui/ring_test.go
git commit -m "feat(tui): кольцевой буфер отладки со счётчиком вытесненного"
```

---

### Task 7: Транскрипт: событие в строки экрана

**Files:**
- Create: `tui/transcript.go`, `tui/transcript_test.go`

**Interfaces:**
- Consumes: `cli.Event`, `cli.EventKind`, `cli.PlayerName` из Task 1–2.
- Produces: `tui.Transcript`, `(*Transcript).Append(cli.Event)`, `(*Transcript).Render(width int) string`.

- [ ] **Step 1: Написать падающий тест**

Создать `tui/transcript_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cli"
)

// Метка ставится при СМЕНЕ говорящего: подпись над каждой строкой подряд
// идущих реплик одного человека — шум, из которого не видно границ реплики.
func TestTranscriptLabelsOnSpeakerChange(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: "Берн", Text: "— Первое.\n"})
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: "Берн", Text: "— Второе.\n"})
	tr.Append(cli.Event{Kind: cli.EventSpeech, Speaker: cli.PlayerName, Text: "— Третье.\n"})

	out := tr.Render(60)
	if got := strings.Count(out, "Берн"); got != 1 {
		t.Errorf("метка «Берн» встречается %d раз, ждали одну", got)
	}
	if !strings.Contains(out, cli.PlayerName) {
		t.Errorf("смена говорящего не отмечена:\n%s", out)
	}
}

// Проза Мастера подписана Мастером: игрок должен видеть, что это не персонаж.
func TestTranscriptLabelsMaster(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventProse, Text: "Дождь бьёт по доскам.\n"})
	if !strings.Contains(tr.Render(60), "Мастер") {
		t.Errorf("проза не подписана Мастером:\n%s", tr.Render(60))
	}
}

// Справка и служебные строки подписи не получают: подписывать «Мастером»
// список команд — враньё об авторстве.
func TestTranscriptLeavesSystemUnsigned(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventSystem, Text: "команды: look, facts\n"})
	if strings.Contains(tr.Render(60), "Мастер") {
		t.Errorf("служебный вывод подписан автором:\n%s", tr.Render(60))
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./tui/ -run Transcript`
Expected: FAIL — `undefined: Transcript`.

- [ ] **Step 3: Реализовать**

Создать `tui/transcript.go`:

```go
package tui

import (
	"strings"

	"github.com/kliuchnikovv/dnd/cli"
)

// MasterName — подпись прозы. Мастер такой же голос, как персонаж, и игрок
// обязан видеть, кто именно с ним говорит.
const MasterName = "Мастер"

// Transcript — что уже сказано, в виде, готовом к показу. Копит события, а не
// строки: подпись и разделитель зависят от автора, а не от текста.
type Transcript struct {
	events []cli.Event
}

func (t *Transcript) Append(e cli.Event) { t.events = append(t.events, e) }

// Render собирает транскрипт под ширину экрана. Метка ставится при смене
// говорящего: подпись над каждой строкой одного и того же человека — шум.
func (t *Transcript) Render(width int) string {
	var b strings.Builder
	prev := ""
	for _, e := range t.events {
		who := speakerOf(e)
		if who != "" && who != prev {
			if b.Len() > 0 {
				b.WriteString(separator(width) + "\n")
			}
			b.WriteString(who + "\n")
		}
		for _, line := range strings.Split(strings.TrimRight(e.Text, "\n"), "\n") {
			b.WriteString("  " + line + "\n")
		}
		prev = who
	}
	return b.String()
}

// speakerOf — кто автор события. Пусто у служебного вывода: подписывать
// список команд чьим-то именем значит врать об авторстве.
func speakerOf(e cli.Event) string {
	switch e.Kind {
	case cli.EventSpeech:
		return e.Speaker
	case cli.EventProse, cli.EventScene:
		return MasterName
	default:
		return ""
	}
}

func separator(width int) string {
	if width < 8 {
		width = 8
	}
	return strings.Repeat("─", width-2)
}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./tui/ && go vet ./tui/`
Expected: PASS.

- [ ] **Step 5: Коммит**

```bash
git add tui/transcript.go tui/transcript_test.go
git commit -m "feat(tui): транскрипт с метками говорящего и разделителями"
```

---

### Task 8: Экран bubbletea

**Files:**
- Create: `tui/model.go`, `tui/run.go`
- Modify: `go.mod`, `go.sum`
- Test: `tui/model_test.go`

**Interfaces:**
- Consumes: `tui.History` (Task 5), `tui.Ring` (Task 6), `tui.Transcript` (Task 7), `(*cli.Session).Start/Feed` (Task 4), `cli.Sink` (Task 1).
- Produces: `tui.Options{Title string, Status func() string, Debug *Ring}`, `tui.Run(s *cli.Session, opts Options) error`.

- [ ] **Step 1: Поставить зависимости**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go mod tidy
```

- [ ] **Step 2: Написать падающий тест**

Создать `tui/model_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kliuchnikovv/dnd/cli"
)

// Ввод уходит в игру по Enter и попадает в историю: без этого стрелка вверх
// пуста, а ради неё всё и затевалось.
func TestEnterSendsInputAndRemembersIt(t *testing.T) {
	m := newModel(nil, Options{})
	m.input.SetValue("осмотреться")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.input.Value() != "" {
		t.Errorf("строка ввода не очищена: %q", m.input.Value())
	}
	if got, ok := m.history.Prev(); !ok || got != "осмотреться" {
		t.Errorf("ввод не попал в историю: %q (%v)", got, ok)
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
```

- [ ] **Step 3: Убедиться, что тест падает**

Run: `go test ./tui/ -run 'Enter|Blocked|Transcript|Tab'`
Expected: FAIL — `undefined: newModel`.

- [ ] **Step 4: Реализовать модель**

Создать `tui/model.go`:

```go
package tui

import (
	"fmt"
	"strings"

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

type model struct {
	session *cli.Session
	opts    Options

	transcript Transcript
	view       viewport.Model
	input      textinput.Model
	history    History

	events chan cli.Event
	// busy — ход исполняется. Ввод в это время заблокирован: два хода
	// одновременно ядро не переживёт, а очередь фраз перепутает ответы.
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
	}
}

func (m model) Init() tea.Cmd { return tea.Batch(textinput.Blink, m.start()) }

// start печатает стартовую сцену через тот же приёмник, что и ходы.
func (m model) start() tea.Cmd {
	return func() tea.Msg {
		if m.session != nil {
			m.session.Start()
		}
		return waitEvent(m.events)()
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

	case eventMsg:
		m.transcript.Append(msg.event)
		m.refresh()
		m.view.GotoBottom()
		return m, waitEvent(m.events)

	case doneMsg:
		m.busy = false
		if msg.quit {
			m.quitting = true
			return m, tea.Quit
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
		m.quitting = true
		return m, tea.Quit
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
```

Создать `tui/run.go`:

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kliuchnikovv/dnd/cli"
)

// sink переправляет события игры в интерфейс. Канал, а не прямая запись:
// ход идёт в своей горутине, а трогать модель из неё нельзя.
type sink struct{ ch chan cli.Event }

func (s sink) Emit(e cli.Event) { s.ch <- e }

// Run открывает полноэкранный режим и не возвращается до выхода из игры.
func Run(s *cli.Session, opts Options) error {
	m := newModel(s, opts)
	s.WithSink(sink{ch: m.events})
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
```

- [ ] **Step 5: Прогнать тесты**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Коммит**

```bash
git add go.mod go.sum tui/model.go tui/run.go tui/model_test.go
git commit -m "feat(tui): полноэкранный экран на bubbletea"
```

---

### Task 9: Проводка в `cmd/dnd`

**Files:**
- Modify: `cmd/dnd/main.go`
- Test: `cmd/dnd/tui_test.go`

**Interfaces:**
- Consumes: `tui.Run`, `tui.Options`, `tui.NewRing` (Task 6, 8), `(*cli.Session).Run` (Task 4).
- Produces: флаг `-plain`, функция `fullscreen(in *os.File, out *os.File, plain bool) bool`.

- [ ] **Step 1: Написать падающий тест**

Создать `cmd/dnd/tui_test.go`:

```go
package main

import (
	"os"
	"testing"
)

// Полноэкранный режим включается только на терминале. Пайп, -script и тесты
// идут построчным путём — на нём держится воспроизводимость по (seed, script).
func TestFullscreenOnlyOnTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	if fullscreen(r, w, false) {
		t.Error("полноэкранный режим включился на пайпе")
	}
	if fullscreen(os.Stdin, os.Stdout, true) {
		t.Error("-plain не выключил полноэкранный режим")
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./cmd/dnd/ -run Fullscreen`
Expected: FAIL — `undefined: fullscreen`.

- [ ] **Step 3: Реализовать**

В `cmd/dnd/main.go` добавить флаг рядом с остальными:

```go
	plain := flag.Bool("plain", false,
		"построчный режим на терминале: без полноэкранного окна, как в скрипте")
```

Добавить определение режима:

```go
// fullscreen сообщает, уместен ли полноэкранный режим. Только когда И ввод, И
// вывод — терминал: пайп, -script и тесты обязаны идти построчным путём,
// иначе воспроизводимость начнёт зависеть от способа запуска.
func fullscreen(in, out *os.File, plain bool) bool {
	if plain {
		return false
	}
	return isTerminal(in) && isTerminal(out)
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
```

Заменить финальный запуск:

```go
	if fullscreen(in, os.Stdout, *plain) {
		title := *casePath
		if err := tui.Run(session, tui.Options{
			Title:  title,
			Status: statusOf(gw),
			Debug:  debugRing,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := session.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
```

Где `debugRing` и `statusOf` заводятся в блоке `-nl`:

```go
	var debugRing *tui.Ring
	var gw *llm.Gateway
	…
		if *debugLLM {
			if fullscreen(in, os.Stdout, *plain) {
				// В полноэкранном режиме stderr затирает экран, поэтому дамп
				// уходит в буфер, а панель по Tab его показывает.
				debugRing = tui.NewRing(200)
				gw = gw.WithDebug(debugRing)
			} else {
				gw = gw.WithDebug(os.Stderr)
			}
		}
```

```go
// statusOf — строка шапки: ход и расход. Функция, а не строка, потому что
// шапка перерисовывается, а расход растёт.
func statusOf(gw *llm.Gateway) func() string {
	if gw == nil {
		return func() string { return "" }
	}
	return func() string {
		return fmt.Sprintf("$%.4f", float64(gw.Stats().SpentMicro)/1e6)
	}
}
```

`in` в `main` объявлен как `*os.File` (`os.Stdin` либо открытый файл скрипта) — тип уже подходит.

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Проверить построчный режим вручную**

```bash
printf 'look\nquit\n' | go run ./cmd/dnd --case cases/harbour/case.json
```

Expected: вывод такой же, как до всей работы (пайп → построчный режим).

- [ ] **Step 6: Коммит**

```bash
git add cmd/dnd/main.go cmd/dnd/tui_test.go
git commit -m "feat(cmd): полноэкранный режим на терминале, -plain для построчного"
```

---

### Task 10: Живая приёмка и настройка

Не автоматизируется — играется руками.

- [ ] **Step 1: Прогнать «Гавань» вживую**

```bash
go run ./cmd/dnd --case cases/harbour/case.json --nl --provider openrouter --model anthropic/claude-sonnet-5 --model-cheap anthropic/claude-haiku-4.5 --price-in 2 --price-out 10 --price-in-cheap 1 --price-out-cheap 5 --cap-day 0.30 --debug-llm
```

Проверить по списку: каретка ходит по строке (`←/→`, `Home/End`, `Ctrl-A/E/W/U`); `↑/↓` поднимают прошлые фразы; `PgUp/PgDn` прокручивают транскрипт и он не прыгает вниз, пока идёт чтение; метки говорящего меняются вместе с автором; разделитель стоит между сменами говорящего; во время хода внизу видно «ход идёт…» и ввод не принимается; `Tab` показывает дамп обмена и возвращает назад; изменение размера окна не ломает раскладку; `Ctrl-C` выходит без мусора на экране.

- [ ] **Step 2: Проверить обвинение целиком**

Довести дело до обвинения и заполнить четыре слота по одному вводу: приглашение слота видно, набор доходит до конца, развязка печатается.

- [ ] **Step 3: Записать найденное**

Всё, что вылезет, — отдельными задачами. Правки раскладки и подписей коммитить по одной.

---

## Порядок и зависимости

Задачи 1 → 2 → 3 → 4 идут строго по порядку: каждая опирается на предыдущую. Задачи 5, 6, 7 независимы друг от друга и от 1–4 (кроме того, что 7 использует типы из 1–2) — их можно делать параллельно. Задача 8 требует 5, 6, 7 и 4. Задача 9 требует 8. Задача 10 — после всех.
