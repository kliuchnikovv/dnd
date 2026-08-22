package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

func transcript(t *testing.T, seed int64, script string) string {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	g := core.NewGame(*cfg)

	in := strings.NewReader(script)
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	return out.String()
}

func TestScriptModeRunsEveryLine(t *testing.T) {
	g := renderGame(t)
	in := strings.NewReader("look\nfacts\nstate\nquit\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("прогон не дал вывода")
	}
}

func TestUnknownCommandDoesNotStopTheRun(t *testing.T) {
	g := renderGame(t)
	in := strings.NewReader("interrogate ivar\nfacts\nquit\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("неизвестная команда уронила прогон: %v", err)
	}
	if !strings.Contains(out.String(), "нельзя") {
		t.Errorf("нет сообщения об отказе: %q", out.String())
	}
}

// TestScriptedAccuseConsumesItsSlotLines регрессионный тест на общий сканер:
// Session.accuse() раньше заводил свой bufio.Scanner поверх того же s.In, что
// уже читал Run(), и это заставляло accuse() терять все четыре строки со
// значениями слотов (первый Scan() в Run() вычерпывал остаток скрипта в свой
// внутренний буфер). Скрипт ниже гонит `accuse` через реальный REPL-цикл и
// проверяет, что все четыре ответа дошли и обвинение подтвердилось.
func TestScriptedAccuseConsumesItsSlotLines(t *testing.T) {
	g := renderGame(t)
	in := strings.NewReader("accuse\ntoke\ncord\nnight\naudit\nquit\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Обвинение верно.") {
		t.Errorf("верное обвинение не распознано: %q", got)
	}
	for _, slot := range []string{"who:", "how:", "when:", "why:"} {
		if !strings.Contains(got, slot) {
			t.Errorf("слот %q не был запрошен: %q", slot, got)
		}
	}
}

func TestSameSeedSameTranscript(t *testing.T) {
	// Пара (seed, скрипт) полностью задаёт вывод — это и есть харнесс.
	script := "look\nexamine body\nfacts\nquit\n"
	first := transcript(t, 7, script)
	second := transcript(t, 7, script)
	if first != second {
		t.Error("один seed дал разные транскрипты")
	}
	if other := transcript(t, 8, script); other == first {
		t.Log("разные seed дали одинаковый транскрипт — допустимо на коротком скрипте")
	}
}

func TestPartialMoveStillArrives(t *testing.T) {
	// Таксономия move: ЧАСТИЧНО — «попал + ухудшение позиции». Игрок обязан
	// оказаться в новом узле, иначе цена прихода берётся без прихода.
	//
	// Проверяется правило презентации, а не бросок: сам move_zone в M1a не
	// бросается вовсе, но flee и всё, что придёт с боем в M1b, пойдут этим же
	// путём.
	g := renderGame(t)
	start := g.Node
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader(""), &out)
	partial := core.TurnResult{Res: &core.Resolution{Class: core.OutcomePartial}}
	s.afterAction(core.Intent{Verb: "move_zone", Args: core.Args{Node: "n_forge"}}, partial)

	if g.Node == start {
		t.Errorf("на ЧАСТИЧНО игрок остался в %s — цена взята без прихода", start)
	}
}

func TestFailedMoveDoesNotArrive(t *testing.T) {
	g := renderGame(t)
	start := g.Node
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader(""), &out)
	fail := core.TurnResult{Res: &core.Resolution{Class: core.OutcomeFail}}
	s.afterAction(core.Intent{Verb: "move_zone", Args: core.Args{Node: "n_forge"}}, fail)

	if g.Node != start {
		t.Errorf("на ПРОВАЛЕ игрок всё равно дошёл до %s", g.Node)
	}
}

// --- переводчик свободного текста ---

type fakeInterp struct {
	intent  *core.Intent
	clarify string
	err     error
	seen    []string
	// with, pending — контекст разговора, доехавший до разбора.
	with    []store.EntityID
	pending []string
}

func (f *fakeInterp) Interpret(_ context.Context, text string, with store.EntityID,
	pending string) (*core.Intent, string, error) {
	f.seen = append(f.seen, text)
	f.with = append(f.with, with)
	f.pending = append(f.pending, pending)
	return f.intent, f.clarify, f.err
}

func runWith(t *testing.T, interp Interpreter, script string) (string, *core.Game) {
	t.Helper()
	g := renderGame(t)
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader(script), &out)
	if interp != nil {
		s.WithInterpreter(interp)
	}
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	return out.String(), g
}

// Без переводчика поведение прежнее: структурированный ввод и отказ.
func TestUnparsedInputStillRefusedWithoutInterpreter(t *testing.T) {
	out, _ := runWith(t, nil, "поболтать с кузнецом о погоде\nquit\n")
	if !strings.Contains(out, "нельзя") {
		t.Errorf("нет отказа: %q", out)
	}
}

func TestInterpreterTurnsFreeTextIntoAction(t *testing.T) {
	fi := &fakeInterp{intent: &core.Intent{Verb: "look"}}
	out, _ := runWith(t, fi, "оглядываюсь по сторонам\nquit\n")
	if strings.Contains(out, "нельзя") {
		t.Errorf("переводчик не подхватил ввод: %q", out)
	}
	if len(fi.seen) != 1 || fi.seen[0] != "оглядываюсь по сторонам" {
		t.Errorf("переводчик получил %v", fi.seen)
	}
}

// Структурированный ввод обязан идти напрямую: он детерминирован, и на нём
// держится воспроизводимость. Переводчик к нему не привлекается.
func TestStructuredInputBypassesInterpreter(t *testing.T) {
	fi := &fakeInterp{intent: &core.Intent{Verb: "look"}}
	runWith(t, fi, "look\nfacts\nstate\nquit\n")
	if len(fi.seen) != 0 {
		t.Errorf("переводчик вызван на структурированном вводе: %v", fi.seen)
	}
}

func TestInterpreterClarificationIsShown(t *testing.T) {
	fi := &fakeInterp{clarify: "К кузнецу или к стражнику?"}
	out, _ := runWith(t, fi, "спрошу его\nquit\n")
	if !strings.Contains(out, "К кузнецу или к стражнику?") {
		t.Errorf("вопрос не показан: %q", out)
	}
}

// Сбой канала не должен выглядеть как отказ мира.
func TestInterpreterFailureIsDistinguishedFromRefusal(t *testing.T) {
	fi := &fakeInterp{err: errors.New("потолок расхода")}
	out, _ := runWith(t, fi, "что-нибудь непонятное\nquit\n")
	if !strings.Contains(out, "переводчик недоступен") {
		t.Errorf("сбой канала подан как отказ мира: %q", out)
	}
	if !strings.Contains(out, "потолок расхода") {
		t.Errorf("причина сбоя скрыта: %q", out)
	}
}

// Перемещение через переводчик обязано работать так же, как через команду.
func TestInterpretedMoveChangesNode(t *testing.T) {
	fi := &fakeInterp{intent: &core.Intent{Verb: "move_zone",
		Args: core.Args{Node: "n_forge"}}}
	_, g := runWith(t, fi, "пойду в кузницу\nquit\n")
	if g.Node != "n_forge" {
		t.Errorf("узел %q — переводчик и команда ведут себя по-разному", g.Node)
	}
}

// --- голос NPC ---

type fakeVoicer struct {
	line  string
	err   error
	calls int
}

func (f *fakeVoicer) Voice(_ context.Context, _ core.Intent, _ core.TurnResult) (string, error) {
	f.calls++
	return f.line, f.err
}

func TestVoicerLinePrintedAfterTurn(t *testing.T) {
	fv := &fakeVoicer{line: "Мокро сегодня."}
	g := renderGame(t)
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("talk_to toke\nquit\n"), &out).WithVoicer(fv)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "— Мокро сегодня.") {
		t.Errorf("реплика не напечатана: %q", out.String())
	}
}

// Отказ ход не тратит, значит и модель звать незачем.
func TestVoicerNotCalledOnRefusal(t *testing.T) {
	fv := &fakeVoicer{line: "не должно прозвучать"}
	g := renderGame(t)
	var out bytes.Buffer
	NewSession(g, strings.NewReader("talk_to призрак\nquit\n"), &out).WithVoicer(fv).Run()
	if fv.calls != 0 {
		t.Errorf("голос вызван на отказе (%d раз)", fv.calls)
	}
}

// Озвучка необязательна: её сбой не должен прерывать ход, который уже прошёл.
func TestVoicerFailureDoesNotBreakTurn(t *testing.T) {
	fv := &fakeVoicer{err: errors.New("потолок расхода")}
	g := renderGame(t)
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("talk_to toke\nfacts\nquit\n"), &out).WithVoicer(fv)
	if err := s.Run(); err != nil {
		t.Fatalf("сбой озвучки уронил прогон: %v", err)
	}
	if !strings.Contains(out.String(), "персонаж промолчал") {
		t.Errorf("сбой не показан: %q", out.String())
	}
	if !strings.Contains(out.String(), "0.500") {
		t.Errorf("ход после сбоя озвучки не продолжился: %q", out.String())
	}
}

func TestWithoutVoicerNothingChanges(t *testing.T) {
	g := renderGame(t)
	var out bytes.Buffer
	if err := NewSession(g, strings.NewReader("talk_to toke\nquit\n"), &out).Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "—") {
		t.Errorf("без озвучки появилась прямая речь: %q", out.String())
	}
}

// Перевод и озвучка одной строки ввода — один ход: иначе потолок вызовов на
// ход считается по разным ведрам и не ограничивает ничего.
func TestInterpretAndVoiceShareOneTurn(t *testing.T) {
	var seen []string
	fi := &fakeInterp{intent: &core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_toke"}}}
	fv := &turnRecordingVoicer{seen: &seen, line: "Да?"}
	g := renderGame(t)
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("поздороваться\nпоздороваться\nquit\n"), &out).
		WithInterpreter(fi).WithVoicer(fv)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("озвучек %d, ожидалось 2", len(seen))
	}
	if seen[0] == seen[1] {
		t.Errorf("две строки ввода получили один номер хода: %v", seen)
	}
	if seen[0] == "" {
		t.Error("номер хода не проставлен — потолок на ход не применится")
	}
}

type turnRecordingVoicer struct {
	seen *[]string
	line string
}

func (v *turnRecordingVoicer) Voice(ctx context.Context, _ core.Intent, _ core.TurnResult) (string, error) {
	*v.seen = append(*v.seen, llm.TurnIDFrom(ctx))
	return v.line, nil
}

// Реплика в воздух — не реплика: персонаж должен понимать, что обращаются к
// нему, иначе он не ответит.
func TestSpeechGetsAddresseeByName(t *testing.T) {
	g := renderGame(t)
	var out bytes.Buffer
	fv := &intentRecordingVoicer{}
	s := NewSession(g, strings.NewReader("«Токе, что слышно?»\nquit\n"), &out).WithVoicer(fv)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if fv.last.Args.Target != "e_toke" {
		t.Errorf("собеседник %q, ожидался e_toke", fv.last.Args.Target)
	}
	if fv.last.Args.Text != "Токе, что слышно?" {
		t.Errorf("сказанное %q", fv.last.Args.Text)
	}
}

// Если в сцене один человек, обращение к нему очевидно и спрашивать нечего.
func TestSpeechFallsBackToSoleNPC(t *testing.T) {
	g := renderGame(t)
	var out bytes.Buffer
	fv := &intentRecordingVoicer{}
	NewSession(g, strings.NewReader("«Что нового?»\nquit\n"), &out).WithVoicer(fv).Run()
	if fv.last.Args.Target != "e_toke" {
		t.Errorf("собеседник %q — в сцене один NPC, он и адресат", fv.last.Args.Target)
	}
}

type intentRecordingVoicer struct{ last core.Intent }

func (v *intentRecordingVoicer) Voice(_ context.Context, in core.Intent, _ core.TurnResult) (string, error) {
	v.last = in
	return "Ничего нового.", nil
}

// Ровно тот случай: в сцене двое, имени в реплике нет — раньше персонаж молчал
// и игрок не понимал, сработало ли что-нибудь.
func TestSpeechAsksWhenAddresseeAmbiguous(t *testing.T) {
	g := twoNPCGame(t)
	var out bytes.Buffer
	fv := &intentRecordingVoicer{}
	NewSession(g, strings.NewReader("«Что за труп?»\nquit\n"), &out).WithVoicer(fv).Run()
	if !strings.Contains(out.String(), "к кому ты обращаешься") {
		t.Errorf("вместо вопроса тишина: %q", out.String())
	}
	if fv.last.Verb != "" {
		t.Error("ход дошёл до движка без адресата")
	}
}

// Разговор продолжается с тем же человеком: назвав его однажды, игрок не
// обязан повторять имя в каждой реплике.
func TestConversationRemembersAddressee(t *testing.T) {
	g := twoNPCGame(t)
	var out bytes.Buffer
	fv := &intentRecordingVoicer{}
	NewSession(g, strings.NewReader("«Берн, что слышно?»\n«А труп?»\nquit\n"), &out).
		WithVoicer(fv).Run()
	if strings.Contains(out.String(), "к кому ты обращаешься") {
		t.Errorf("вторая реплика потеряла собеседника: %q", out.String())
	}
	if fv.last.Args.Target != "e_bern" {
		t.Errorf("вторая реплика ушла к %q, ожидался e_bern", fv.last.Args.Target)
	}
}

// Знак вопроса не стоит вызова модели.
func TestMeaninglessInputCostsNoModelCall(t *testing.T) {
	fi := &fakeInterp{intent: &core.Intent{Verb: "look"}}
	g := renderGame(t)
	var out bytes.Buffer
	NewSession(g, strings.NewReader("?\n...\nquit\n"), &out).WithInterpreter(fi).Run()
	if len(fi.seen) != 0 {
		t.Errorf("модель вызвана на мусорном вводе: %v", fi.seen)
	}
	if !strings.Contains(out.String(), "не понял") {
		t.Errorf("игроку не сказано, что ввод не понят: %q", out.String())
	}
}

// Свободный текст через модель обязан проходить тем же путём, что команда:
// иначе адресат теряется в одном из режимов.
func TestInterpretedSpeechAlsoGetsAddressee(t *testing.T) {
	g := twoNPCGame(t)
	fi := &fakeInterp{intent: &core.Intent{Verb: "say",
		Args: core.Args{Text: "Берн, что слышно?"}}}
	fv := &intentRecordingVoicer{}
	var out bytes.Buffer
	NewSession(g, strings.NewReader("скажу Берну пару слов\nquit\n"), &out).
		WithInterpreter(fi).WithVoicer(fv).Run()
	if fv.last.Args.Target != "e_bern" {
		t.Errorf("адресат %q — свободный текст не прошёл разрешение", fv.last.Args.Target)
	}
}

// twoNPCGame — сцена с двумя людьми: без неё двусмысленность не проверить.
func twoNPCGame(t *testing.T) *core.Game {
	t.Helper()
	g := renderGame(t)
	g.DB.Entities["e_bern"] = store.Entity{ID: "e_bern", Name: "Берн, стражник",
		Kind: store.EntityNPC, Voice: "сухой", Node: g.Node}
	g.DB.Entities["e_nils"] = store.Entity{ID: "e_nils", Name: "Нильс, посыльный",
		Kind: store.EntityNPC, Voice: "торопливый", Node: g.Node}
	delete(g.DB.Entities, "e_toke")
	return g
}

// Ровно тот ввод: адресат назван вне кавычек, вопрос внутри.
func TestAddresseeFromTextOutsideQuotes(t *testing.T) {
	g := twoNPCGame(t)
	fv := &intentRecordingVoicer{}
	var out bytes.Buffer
	NewSession(g, strings.NewReader(`Обращаясь к Нильсу - "А ты ничего не видел?"`+"\nquit\n"), &out).
		WithVoicer(fv).Run()
	if fv.last.Args.Target != "e_nils" {
		t.Errorf("адресат %q, ожидался e_nils", fv.last.Args.Target)
	}
	if fv.last.Args.Text != "А ты ничего не видел?" {
		t.Errorf("сказано %q", fv.last.Args.Text)
	}
	if strings.Contains(out.String(), "о чём именно спросить") {
		t.Errorf("речь ушла в question вместо say: %q", out.String())
	}
}

func TestSurveyIsAvailableAndFree(t *testing.T) {
	out := transcript(t, 1, "survey\nclocks\nquit\n")
	if !strings.Contains(out, "Ящики у стены") {
		t.Errorf("survey не показал проп: %q", out)
	}
	if !strings.Contains(out, "0/6") {
		t.Errorf("свободная проба сдвинула часы: %q", out)
	}
}

// Сцена по прибытии — тот же честный список, что и survey. Печатать здесь
// одних холдеров значило бы выдавать граф каждым переходом.
func TestSceneListsPropsAlongsideHolders(t *testing.T) {
	out := transcript(t, 1, "look\nquit\n")
	if !strings.Contains(out, "Ящики у стены") {
		t.Errorf("сцена не показала проп: %q", out)
	}
}

// DoD M1a — «до прочитанной развязки». Верное обвинение обязано напечатать
// речь игрока и последствия, а не строку статуса.
func TestCorrectAccusationPrintsSummationAndAftermath(t *testing.T) {
	out := transcript(t, 1, "accuse\ntoke\ncord\nnight\naudit\nquit\n")
	for _, want := range []string{"шнур", "Токе увели"} {
		if !strings.Contains(out, want) {
			t.Errorf("в развязке нет %q:\n%s", want, out)
		}
	}
}

// Ошибочное обвинение развязки не печатает: игра не объясняет решение тому,
// кто его не нашёл.
func TestWrongAccusationPrintsNoAftermath(t *testing.T) {
	out := transcript(t, 1, "accuse\ntoke\ncord\nnight\ncord\nquit\n")
	if strings.Contains(out, "Токе увели") {
		t.Errorf("развязка напечатана при ошибке:\n%s", out)
	}
}

func TestStalledCaseEndsWithColdCaseText(t *testing.T) {
	g := renderGame(t)
	g.C.Tick("c_suspicion", 6)
	in := strings.NewReader("look\nfacts\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if !strings.Contains(out.String(), "Прилив пришёл и ушёл") {
		t.Errorf("висяк закончился молчанием:\n%s", out.String())
	}
	if strings.Contains(out.String(), "подтверждён тремя") {
		t.Errorf("прогон продолжился после висяка:\n%s", out.String())
	}
}

func TestRunEndsOnCorrectAccusation(t *testing.T) {
	out := transcript(t, 1, "accuse\ntoke\ncord\nnight\naudit\nfacts\n")
	if strings.Contains(out, "подтверждён тремя") {
		t.Errorf("прогон продолжился после развязки:\n%s", out)
	}
}

// --- контекст разговора для разбора ---

// Игра спросила — значит следующая фраза игрока это ответ, и разбор обязан
// знать вопрос. Живой прогон зациклился ровно здесь: «чем именно?» → «рукой»
// → снова «уточни, что именно ты делаешь».
func TestPendingQuestionReachesTheNextParse(t *testing.T) {
	fi := &fakeInterp{clarify: "чем именно?"}
	out, _ := runWith(t, fi, "достаю предписание\nрукой\nquit\n")

	if len(fi.pending) < 2 {
		t.Fatalf("разбор вызван %d раз: %v", len(fi.pending), fi.seen)
	}
	if fi.pending[0] != "" {
		t.Errorf("первый ввод пришёл с вопросом из ниоткуда: %q", fi.pending[0])
	}
	if fi.pending[1] != "чем именно?" {
		t.Errorf("заданный вопрос не доехал до следующего разбора: %q", fi.pending[1])
	}
	if !strings.Contains(out, "чем именно?") {
		t.Errorf("вопрос не показан игроку:\n%s", out)
	}
}

// Ответ пришёл — вопрос закрыт. Иначе старый контекст навязывается модели
// сколько угодно ходов подряд.
func TestAnsweredQuestionIsNotAskedAgain(t *testing.T) {
	fi := &fakeInterp{intent: &core.Intent{Verb: "look"}}
	fi.clarify = ""
	runWith(t, fi, "осмотреться\nещё раз осмотреться\nquit\n")

	for i, p := range fi.pending {
		if p != "" {
			t.Errorf("разбор %d получил вопрос, которого не задавали: %q", i, p)
		}
	}
}

// Собеседник доезжает до разбора: разговор идёт с ним, и «спрошу его же»
// иначе не разобрать.
func TestInterlocutorReachesTheParse(t *testing.T) {
	fi := &fakeInterp{intent: &core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_toke"}}}
	runWith(t, fi, "поздороваться с Токе\nа что нового\nquit\n")

	if len(fi.with) < 2 {
		t.Fatalf("разбор вызван %d раз", len(fi.with))
	}
	if fi.with[1] != "e_toke" {
		t.Errorf("собеседник не доехал: %q", fi.with[1])
	}
}

// Спросить об инвентаре словами так же законно, как командой: игрок не обязан
// знать, что одно спрашивается фразой, а другое — командой. И вызов модели на
// это тратить незачем — ответ у игры уже есть.
func TestAskingAboutInventoryInWordsShowsIt(t *testing.T) {
	fi := &fakeInterp{clarify: "не должно понадобиться"}
	g := renderGame(t)
	g.DB.Items["i_writ"] = store.Item{ID: "i_writ", Kind: "credential", Name: "Предписание магистрата"}
	g.Acquire("i_writ")

	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("что у меня в карманах?\nquit\n"), &out).WithInterpreter(fi)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Предписание магистрата") {
		t.Errorf("инвентарь не показан:\n%s", out.String())
	}
	if len(fi.seen) != 0 {
		t.Errorf("на вопрос об инвентаре потрачен переводчик: %v", fi.seen)
	}
}

// «Достаю из кармана» — это действие, а не вопрос о карманах: такую фразу
// разбирает переводчик, иначе игра отвечает списком вместо действия.
func TestReachingIntoAPocketIsNotAnInventoryQuery(t *testing.T) {
	fi := &fakeInterp{clarify: "что вы достаёте?"}
	var out bytes.Buffer
	s := NewSession(renderGame(t), strings.NewReader("достаю из кармана\nquit\n"), &out).
		WithInterpreter(fi)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if len(fi.seen) != 1 {
		t.Errorf("фраза не дошла до переводчика: %v", fi.seen)
	}
}

// Открытый вопрос тому, кому нечего сказать, отказом не кончается: ход
// исполняется, игрок видит прозу, а не «нельзя». Сухой отказ на попытку
// заговорить — то самое «свидетели молчат», из-за которого плейтесты решали,
// что механика им недоступна.
func TestUnanswerableOpenQuestionDoesNotRefuse(t *testing.T) {
	g := renderGame(t)
	var out bytes.Buffer
	// e_toke в minimal.json фактов по гейту question не держит.
	s := NewSession(g, strings.NewReader("question toke\nquit\n"), &out)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "нельзя") {
		t.Errorf("открытый вопрос кончился отказом:\n%s", out.String())
	}
}
