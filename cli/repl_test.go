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
	"github.com/kliuchnikovv/dnd/rules/threshold"
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
	g := renderGame(t)
	start := g.Node
	var out bytes.Buffer
	// d20=10 при edge+1 против порога 14 даёт маржу -3, то есть ЧАСТИЧНО.
	g.Dice = dice.Fixed(10)
	in := strings.NewReader("move_zone forge\nquit\n")
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ЧАСТИЧНО") {
		t.Fatalf("бросок дал не ЧАСТИЧНО, тест не о том:\n%s", out.String())
	}
	if g.Node == start {
		t.Errorf("на ЧАСТИЧНО игрок остался в %s — цена взята без прихода", start)
	}
}

// --- переводчик свободного текста ---

type fakeInterp struct {
	intent  *core.Intent
	clarify string
	err     error
	seen    []string
}

func (f *fakeInterp) Interpret(_ context.Context, text string) (*core.Intent, string, error) {
	f.seen = append(f.seen, text)
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
	fv := &fakeVoicer{line: "— Мокро сегодня."}
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
	fv := &fakeVoicer{line: "— не должно прозвучать"}
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
