package cli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
)

// Вариант читается словами игрока, а не идентификаторами. Имена берутся из тех
// же таблиц, что список целей: разойдись они, игрок читал бы про «e_toke» там,
// где сцена печатает «Токе».
func TestAffordancesReadAsWords(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Affordances(g, g.Affordances(""), nil)
	if strings.Contains(out, "e_toke") || strings.Contains(out, "p_crates") {
		t.Errorf("в списке идентификаторы вместо имён:\n%s", out)
	}
	if !strings.Contains(out, "Токе") {
		t.Errorf("присутствующий не назван по имени:\n%s", out)
	}
	if !strings.Contains(out, "1.") {
		t.Errorf("список не нумерован:\n%s", out)
	}
}

// Свободный ввод равноправен, и игрок обязан это видеть. Без этой строки список
// читается как закрытое меню — ровно тот тупик, из которого ветка выбиралась.
func TestAffordanceListOffersFreeText(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Affordances(g, g.Affordances(""), nil)
	if !strings.Contains(out, "своими словами") {
		t.Errorf("список выглядит закрытым меню:\n%s", out)
	}
}

// Ни одна строка-ДЕЙСТВИЕ не начинается с тире и не берётся в кавычки: такая
// строка разбирается как прямая речь, и перепечатанный игроком вариант ушёл бы
// в say вместо осмотра.
//
// К репликам это не относится намеренно. Для реплики слова И ЕСТЬ то, что
// игрок сказал бы, и фраза, ушедшая в say, — связный исход, а не поломка.
func TestActionLinesAreNotMistakenForSpeech(t *testing.T) {
	g := renderGame(t)
	list := g.Affordances("")
	for i, line := range strings.Split(Render{}.Affordances(g, list, nil), "\n") {
		if _, _, ok := Speech(line); ok {
			t.Errorf("строка %d разбирается как речь: %q", i, line)
		}
	}
}

// Реплики печатаются в кавычках, действия — нет: игрок должен видеть, где он
// говорит, а где делает.
func TestRepliesAreQuotedAndActionsAreNot(t *testing.T) {
	g := renderGame(t)
	list := []core.Affordance{
		{Intent: core.Intent{Verb: "ask_about", Args: core.Args{Target: "e_toke"}}, Reply: true},
		{Intent: core.Intent{Verb: "examine", Args: core.Args{Target: "p_crates"}}},
	}
	out := Render{}.Affordances(g, list, []string{"Как тут живут?", "осмотреть ящики"})
	if !strings.Contains(out, "«Как тут живут?»") {
		t.Errorf("реплика без кавычек:\n%s", out)
	}
	if strings.Contains(out, "«осмотреть ящики»") {
		t.Errorf("действие взято в кавычки:\n%s", out)
	}
}

// Кавычки ставит презентация, а не Мастер: две пары кавычек подряд — это
// сломанная строка, и увидит её игрок, а не тест Мастера.
func TestQuotesAreNotDoubled(t *testing.T) {
	g := renderGame(t)
	list := []core.Affordance{
		{Intent: core.Intent{Verb: "ask_about", Args: core.Args{Target: "e_toke"}}, Reply: true},
	}
	out := Render{}.Affordances(g, list, []string{"«Как тут живут?»"})
	if strings.Contains(out, "««") {
		t.Errorf("кавычки удвоены:\n%s", out)
	}
}

// Тег несёт класс проверки и никогда порог. Число живёт в гейте держателя, то
// есть в данных дела: напечатать его значило бы разметить авторские цели.
func TestCheckTagCarriesClassNotThreshold(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Affordances(g, g.Affordances(""), nil)
	for _, n := range []string{"10", "14", "18"} {
		if strings.Contains(out, n) {
			t.Errorf("в списке порог %s:\n%s", n, out)
		}
	}
	if !strings.Contains(out, "[расследование]") {
		t.Errorf("у бросаемого варианта нет тега класса:\n%s", out)
	}
}

// Пустой набор печатается пустой строкой, а не заголовком без списка.
func TestEmptyAffordancesPrintNothing(t *testing.T) {
	if out := (Render{}).Affordances(renderGame(t), nil, nil); out != "" {
		t.Errorf("пустой набор напечатал %q", out)
	}
}

// Каждый класс из реестра переведён. Непереведённый класс потерял бы тег молча:
// игрок увидел бы вариант без пометки о броске.
func TestEveryRollingClassHasAWord(t *testing.T) {
	for _, def := range core.AllVerbs() {
		if !def.Rolls {
			continue
		}
		if _, ok := classWords[def.Class]; !ok {
			t.Errorf("класс %q без перевода: %s остался бы без тега", def.Class, def.Verb)
		}
	}
}

// Номер разворачивается в тот же интент, что и написанный словами вариант. Два
// пути к одному ходу разошлись бы, и разошлись бы молча.
func TestNumberExpandsIntoTheSameIntent(t *testing.T) {
	// Вариант с осмотром ищется в наборе, а не берётся номером наугад:
	// приоритет категорий — дело ядра, и тест не обязан его повторять.
	g := renderGame(t)
	want, index := core.Intent{}, 0
	for i, a := range g.Affordances("") {
		if a.Intent.Verb == "examine" {
			want, index = a.Intent, i+1
		}
	}
	if index == 0 {
		t.Fatal("в наборе нет осмотра — тест сравнивать не с чем")
	}

	byNumber := affordSession(t, strconv.Itoa(index)+"\nquit\n")
	byWords := affordSession(t, "examine "+string(want.Args.Target)+"\nquit\n")
	if !reflect.DeepEqual(byNumber.first, byWords.first) {
		t.Errorf("номер и слова дали разные ходы:\n%+v\n%+v", byNumber.first, byWords.first)
	}
	if !reflect.DeepEqual(byNumber.first, want) {
		t.Errorf("номер развернулся не в свой вариант:\n%+v\n%+v", byNumber.first, want)
	}
}

// Номер вне диапазона хода не тратит: это опечатка, а не заявка.
func TestOutOfRangeNumberSpendsNoTurn(t *testing.T) {
	run := affordSession(t, "9\nquit\n")
	if run.journalled != 0 {
		t.Errorf("номер вне диапазона попал в журнал команд: %d", run.journalled)
	}
	if !strings.Contains(run.out, "такого варианта нет") {
		t.Errorf("игрок не узнал, что промахнулся:\n%s", run.out)
	}
}

// Без включённого набора номер остаётся обычным вводом: игра без аффордансов
// обязана работать как работала.
func TestNumberIsPlainInputWithoutAffordances(t *testing.T) {
	g := renderGame(t)
	var out strings.Builder
	s := NewSession(g, strings.NewReader("1\nquit\n"), &out)
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if strings.Contains(out.String(), "Что можно:") {
		t.Errorf("набор напечатан без включения:\n%s", out.String())
	}
	if strings.Contains(out.String(), "такого варианта нет") {
		t.Errorf("номер разобран как вариант при выключенном наборе:\n%s", out.String())
	}
}

// Набор печатается каждый ход: он и есть ответ на вопрос «что теперь».
func TestAffordancesArePrintedEveryTurn(t *testing.T) {
	run := affordSession(t, "survey\nsurvey\nquit\n")
	if n := strings.Count(run.out, "Что можно:"); n < 3 {
		t.Errorf("набор напечатан %d раз на старте и двух ходах:\n%s", n, run.out)
	}
}

type affordRun struct {
	out        string
	first      core.Intent
	journalled int
}

// affordSession гоняет сессию с включённым набором и подсматривает журнал
// команд: правда реплея — интент, а не строка, и номер обязан доезжать до неё.
func affordSession(t *testing.T, script string) affordRun {
	t.Helper()
	g := renderGame(t)
	var out strings.Builder
	db := g.DB
	s := NewSession(g, strings.NewReader(script), &out).
		WithJournal(NewJournal(db, "s-afford", "snap", 1)).WithAffordances()
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	run := affordRun{out: out.String()}
	for _, e := range db.CommandLog {
		run.journalled++
		if run.journalled == 1 {
			var cmd loggedCommand
			if err := json.Unmarshal(e.Intent, &cmd); err != nil {
				t.Fatalf("интент из журнала: %v", err)
			}
			run.first = cmd.Intent
		}
	}
	return run
}

// Имя во фразе идёт без пояснения после запятой: «заговорить с Берн, стражник»
// не согласуется — пояснение стоит в именительном. В перечне целей оно
// остаётся: там оно полезно.
func TestNameInPhraseDropsTheApposition(t *testing.T) {
	if got := shortName("Берн, стражник"); got != "Берн" {
		t.Errorf("пояснение не отрезано: %q", got)
	}
	if got := shortName("Штабель бочек"); got != "Штабель бочек" {
		t.Errorf("имя без запятой пострадало: %q", got)
	}
}

// Тег не обещает проверки, которой не будет. Переход системой правил не
// бросается вовсе, и «[перемещение]» читалось бы игроком как заявка на риск.
func TestMoveOptionPromisesNoCheck(t *testing.T) {
	g := renderGame(t)
	for _, a := range g.Affordances("") {
		if a.Intent.Verb != "move_zone" {
			continue
		}
		if label := AffordanceLabel(g, a); strings.Contains(label, "[") {
			t.Errorf("переход обещает проверку: %q", label)
		}
	}
}

// Набор доступен приёмнику структурой, а не только текстом. Полноэкранный
// режим рисует его своей панелью и подсвечивает выбранный вариант: разбирать
// для этого напечатанные строки значило бы парсить собственный вывод.
func TestOfferedIsReadableBySink(t *testing.T) {
	g := renderGame(t)
	s := NewSession(g, strings.NewReader("quit\n"), &strings.Builder{}).WithAffordances()
	s.Start()
	offered := s.Offered()
	if len(offered) == 0 {
		t.Fatal("после старта набор не виден приёмнику")
	}
	if !reflect.DeepEqual(offered, g.Affordances("")) {
		t.Errorf("приёмнику виден не тот набор, что показан:\n%+v\n%+v",
			offered, g.Affordances(""))
	}
}

// Набор идёт своим видом события, а не системным. Приглашение уже отделено
// так же: полноэкранному режиму надо знать, что это не строка транскрипта, а
// панель, которая себя заменяет.
func TestAffordancesHaveTheirOwnEventKind(t *testing.T) {
	g := renderGame(t)
	rec := &recordingSink{}
	s := NewSession(g, strings.NewReader("quit\n"), &strings.Builder{}).
		WithSink(rec).WithAffordances()
	s.Start()
	var found bool
	for _, e := range rec.events {
		if strings.Contains(e.Text, "Что можно:") {
			found = true
			if e.Kind != EventOptions {
				t.Errorf("набор пришёл видом %q", e.Kind)
			}
		}
	}
	if !found {
		t.Errorf("набор не дошёл до приёмника: %+v", rec.events)
	}
}

// Ярлык одного варианта нужен снаружи: панель полноэкранного режима рисует
// строки сама, потому что подсвечивает одну из них.
func TestSingleLabelIsAvailableToRenderers(t *testing.T) {
	g := renderGame(t)
	list := g.Affordances("")
	if len(list) == 0 {
		t.Fatal("набор пуст")
	}
	if label := AffordanceLabel(g, list[0]); label == "" || strings.Contains(label, "e_") {
		t.Errorf("ярлык варианта негоден: %q", label)
	}
}

type recordingSink struct{ events []Event }

func (r *recordingSink) Emit(e Event) { r.events = append(r.events, e) }

type fakeOptionVoicer struct {
	lines []string
	err   error
	// errs — ошибки по вызовам подряд: первый вызов берёт errs[0], второй —
	// errs[1] и так далее. Пустой — используется единый err на все вызовы.
	// Нужно проверить «сбой на одном ходу — успех на следующем» без выдумки
	// второго типа подделки.
	errs  []error
	calls int
	seen  [][]Option
}

func (f *fakeOptionVoicer) VoiceOptions(_ context.Context, opts []Option) ([]string, error) {
	err := f.err
	if f.calls < len(f.errs) {
		err = f.errs[f.calls]
	}
	f.calls++
	f.seen = append(f.seen, opts)
	return f.lines, err
}

// Слова Мастера заменяют кодовые. Без них список читается как перечень команд,
// а не как разговор.
func TestVoicedWordsReplaceTheCodeOnes(t *testing.T) {
	g := renderGame(t)
	// renderGame даёт ровно два варианта (заговорить, осмотреть) — свой набор
	// на этот фикстур, а не выдумка теста: fakeOptionVoicer обязан вернуть
	// столько же строк, иначе применится правило «всё или ничего».
	v := &fakeOptionVoicer{lines: []string{"раз", "два"}}
	var out strings.Builder
	s := NewSession(g, strings.NewReader("quit\n"), &out).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	if got := out.String(); !strings.Contains(got, "раз") {
		t.Errorf("слова Мастера не напечатаны:\n%s", got)
	}
}

// Число не совпало — печатаются кодовые слова, все до одной. Приложить что
// пришло к первым вариантам значило бы подписать строку под чужой интент.
func TestMismatchedVoicingFallsBackWholesale(t *testing.T) {
	g := renderGame(t)
	// Набор renderGame даёт два варианта — здесь строка одна, то есть заведомо
	// мимо.
	v := &fakeOptionVoicer{lines: []string{"раз"}}
	var out strings.Builder
	s := NewSession(g, strings.NewReader("quit\n"), &out).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	got := out.String()
	if strings.Contains(got, "раз") {
		t.Errorf("частичная озвучка применена:\n%s", got)
	}
	if !strings.Contains(got, "заговорить") {
		t.Errorf("откат на кодовые слова не сработал:\n%s", got)
	}
}

// Сбой озвучки ход не рушит: надстройка не должна быть условием работы.
func TestVoicingFailureKeepsTheGameRunning(t *testing.T) {
	g := renderGame(t)
	v := &fakeOptionVoicer{err: errors.New("шлюз закрыт")}
	var out strings.Builder
	s := NewSession(g, strings.NewReader("quit\n"), &out).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	if !strings.Contains(out.String(), "заговорить") {
		t.Errorf("сбой озвучки съел список:\n%s", out.String())
	}
}

// Набор часто повторяется от хода к ходу, и повтор платить не должен.
func TestUnchangedSetIsNotVoicedTwice(t *testing.T) {
	g := renderGame(t)
	v := &fakeOptionVoicer{lines: []string{"раз", "два"}}
	s := NewSession(g, strings.NewReader("survey\nsurvey\nquit\n"), &strings.Builder{}).
		WithAffordances().WithOptionVoicer(v)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if v.calls != 1 {
		t.Errorf("вызовов озвучки %d на неизменившийся набор", v.calls)
	}
}

// Разовый сбой озвучки не должен залипать до смены набора: набор в разговоре
// меняется редко, и без повторной попытки игрок просидит десяток ходов с
// кодовыми словами из-за одной секундной ошибки сети.
func TestVoicingRetriesAfterFailureOnUnchangedSet(t *testing.T) {
	g := renderGame(t)
	v := &fakeOptionVoicer{
		lines: []string{"раз", "два"},
		errs:  []error{errors.New("шлюз закрыт"), nil},
	}
	s := NewSession(g, strings.NewReader("survey\nsurvey\nquit\n"), &strings.Builder{}).
		WithAffordances().WithOptionVoicer(v)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if v.calls != 2 {
		t.Errorf("после сбоя на неизменившемся наборе вызовов %d, хотим 2", v.calls)
	}
	if got := s.offeredWords; len(got) != 2 || got[0] != "раз" {
		t.Errorf("второй ход не получил слова после сбоя на первом: %v", got)
	}
}

// Мастеру видно, где реплика: без этого он напишет действие от первого лица
// или реплику в неопределённой форме.
func TestVoicerIsToldWhichOptionsAreReplies(t *testing.T) {
	g := renderGame(t)
	v := &fakeOptionVoicer{lines: []string{"раз", "два"}}
	s := NewSession(g, strings.NewReader("quit\n"), &strings.Builder{}).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	if len(v.seen) == 0 {
		t.Fatal("озвучка не вызвана")
	}
	for i, o := range v.seen[0] {
		if o.Text == "" {
			t.Errorf("вариант %d ушёл на озвучку без кодовых слов", i)
		}
	}
}

// Слова набора обязаны лечь в аудит: следующий Feed стирает поля предложения
// до того, как офер вызовет журнал через обычный noteProposal, так что строка
// нужна собственная — вне очереди разбора, как у реплики персонажа.
func TestVoicedOptionsReachTheAuditLog(t *testing.T) {
	g := renderGame(t)
	v := &fakeOptionVoicer{lines: []string{"раз", "два"}}
	s := NewSession(g, strings.NewReader("quit\n"), &strings.Builder{}).
		WithJournal(NewJournal(g.DB, "s-afford-audit", "snap", 1)).
		WithAffordances().WithOptionVoicer(v)
	s.Start()
	for _, e := range g.DB.Audit {
		if e.LLMRole != string(llm.RoleOptions) {
			continue
		}
		if strings.Contains(e.LLMProposal, "раз") && strings.Contains(e.LLMProposal, "два") {
			return
		}
	}
	t.Errorf("слова набора не попали в аудит:\n%+v", g.DB.Audit)
}
