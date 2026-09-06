package vignette

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeNarrator — тестовый нарратор: запоминает, ЧТО ему дали (для анти-лик
// проверки), и отдаёт заданную прозу/ошибку.
type fakeNarrator struct {
	out         string
	err         error
	gotRevealed []string
	gotSurfaces []string
	gotAmbient  string
	gotFrame    string
}

func (f *fakeNarrator) Narrate(_ context.Context, ambient string, surfaces, revealed []string, frame string) (string, error) {
	f.gotAmbient, f.gotSurfaces, f.gotRevealed, f.gotFrame = ambient, surfaces, revealed, frame
	return f.out, f.err
}

// Офлайн (нарратор nil): проза — склейка того, что ядро ОТКРЫЛО (revealed +
// нейтральные строки), без правды.
func TestNarrate_OfflineJoinsOutcome(t *testing.T) {
	sc, st := guestScene(), testState()
	res := Result{Revealed: []string{"Голос знаком.", "Пёс рычит."}, StateNote: "Ты у очага."}

	got, err := Narrate(context.Background(), sc, st, res, nil, KeywordGuard{})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{"Голос знаком.", "Пёс рычит.", "Ты у очага."} {
		if !strings.Contains(got, want) {
			t.Fatalf("склейка outcome не содержит %q: %q", want, got)
		}
	}
	if strings.Contains(got, sc.Truth) {
		t.Fatalf("офлайн-проза протекла правдой: %q", got)
	}
}

// Нарратору уходят ТОЛЬКО revealed + нейтральные поверхности — ни правды, ни
// закрытых тиров (анти-лик конструкцией, ADR-0008).
func TestNarrate_NarratorGetsOnlyRevealed(t *testing.T) {
	sc, st := guestScene(), testState()
	fn := &fakeNarrator{out: "Мастер написал прозу."}
	res := Result{Revealed: []string{"Голос знаком."}, Beat: "Голос просит впустить."}

	got, err := Narrate(context.Background(), sc, st, res, fn, KeywordGuard{})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != "Мастер написал прозу." {
		t.Fatalf("проза нарратора не проброшена: %q", got)
	}
	if len(fn.gotRevealed) != 1 || fn.gotRevealed[0] != "Голос знаком." {
		t.Fatalf("нарратор получил не только revealed: %#v", fn.gotRevealed)
	}
	for _, leaked := range append([]string{sc.Truth}, "Оно зовёт по имени и знает, что было днём.") {
		for _, s := range fn.gotSurfaces {
			if strings.Contains(s, leaked) {
				t.Fatalf("нарратор получил защищённое в surfaces: %q", s)
			}
		}
	}
	if fn.gotFrame != "Голос просит впустить." { // frame = firstNonEmpty(Beat, EndText, StateNote, default)
		t.Fatalf("frame не из беата: %q", fn.gotFrame)
	}
}

// Страж режет дословную утечку защищённого факта из черновика Мастера.
func TestNarrate_GuardRedactsVerbatimLeak(t *testing.T) {
	sc, st := guestScene(), testState()
	fn := &fakeNarrator{out: "Мастер сболтнул: " + sc.Truth}
	res := Result{Revealed: []string{"Голос знаком."}}

	got, err := Narrate(context.Background(), sc, st, res, fn, KeywordGuard{})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(got, sc.Truth) {
		t.Fatalf("страж не вырезал дословную правду: %q", got)
	}
}

// Карваут финала: на ended раскрытая развязка (EndText) в allowed и НЕ режется,
// даже если дословно содержит правду.
func TestNarrate_EndedCarveoutKeepsEndText(t *testing.T) {
	sc, st := guestScene(), testState()
	fn := &fakeNarrator{out: sc.Truth} // Мастер выдаёт ровно правду — но это развязка
	res := Result{Ended: true, EndText: sc.Truth}

	got, err := Narrate(context.Background(), sc, st, res, fn, KeywordGuard{})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(got, sc.Truth) {
		t.Fatalf("карваут финала срезал развязку: %q", got)
	}
}

// Ошибка нарратора пробрасывается наружу (сервер печатает errorFrame, CLI — в stderr).
func TestNarrate_NarratorErrorPropagates(t *testing.T) {
	sc, st := guestScene(), testState()
	fn := &fakeNarrator{err: errors.New("бум")}

	got, err := Narrate(context.Background(), sc, st, Result{Revealed: []string{"x"}}, fn, KeywordGuard{})
	if err == nil {
		t.Fatalf("ожидалась ошибка нарратора, got=%q", got)
	}
}
