package main

import (
	"io"
	"os"
	"testing"

	"github.com/kliuchnikovv/dnd/tui"
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

// /dev/null — тоже символьное устройство (os.ModeCharDevice), поэтому старая
// проверка через режим файла отвечала на нём "терминал". `dnd < /dev/null >
// /dev/null` уходил в полноэкранную ветку и падал на open /dev/tty. §8 спека
// обещает, что не-терминал полноэкранным режимом не становится никогда —
// term.IsTerminal обязан честно сказать "нет" и на /dev/null.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if isTerminal(f) {
		t.Error("/dev/null распознан как терминал")
	}
	if fullscreen(f, f, false) {
		t.Error("/dev/null включил полноэкранный режим")
	}
}

// debugSink — куда уходят внештатные сообщения (алерт леджера, «Мастер не
// ответил»): без кольца — в stderr, как построчный режим и раньше; с кольцом
// (полноэкранный режим) — туда, а не поверх альт-экрана.
func TestDebugSinkPicksRingOverStderr(t *testing.T) {
	if debugSink(nil) != os.Stderr {
		t.Error("без кольца debugSink не отдал stderr")
	}
	r := tui.NewRing(4)
	if debugSink(r) != io.Writer(r) {
		t.Error("с кольцом debugSink не отдал его")
	}
}

// noDebugReasonFor выбирает честный текст для панели отладки в трёх разных
// раскладах -nl/-debug-llm/кольца. Раньше -debug-llm без -nl отвечал общим
// «запустите с -debug-llm» тому, кто флаг и указал; а -nl без -debug-llm
// оставлял непустое кольцо (заведённое как приёмник алерта леджера и
// «Мастер не ответил») без единого слова о том, что дампа там не будет.
func TestNoDebugReasonFor(t *testing.T) {
	ring := tui.NewRing(4)
	cases := []struct {
		name         string
		nl, debugLLM bool
		ring         *tui.Ring
		wantNonEmpty bool
	}{
		{"debug-llm без -nl: шлюза моделей нет вовсе", false, true, nil, true},
		{"-nl без -debug-llm, кольцо есть: дампа не будет", true, false, ring, true},
		{"-nl с -debug-llm: дамп подключён, причина не нужна", true, true, ring, false},
		{"ни -nl, ни -debug-llm: полноэкранный режим ни при чём", false, false, nil, false},
		{"-nl без -debug-llm, но кольца нет (не fullscreen)", true, false, nil, false},
	}
	for _, c := range cases {
		got := noDebugReasonFor(c.nl, c.debugLLM, c.ring)
		if (got != "") != c.wantNonEmpty {
			t.Errorf("%s: noDebugReasonFor(%v, %v, ring=%v) = %q", c.name, c.nl, c.debugLLM, c.ring != nil, got)
		}
	}
}

// -nl и -chat вместе — ошибка запуска, а не молчаливый приоритет одного из
// них: два разбора одной фразы это два разных ответа на один ввод, и игрок
// никогда не узнает, какой он получил.
func TestChatAndNLAreMutuallyExclusive(t *testing.T) {
	if err := checkModes(true, true); err == nil {
		t.Error("-nl вместе с -chat прошли молча")
	}
	for _, c := range []struct{ nl, chat bool }{{true, false}, {false, true}, {false, false}} {
		if err := checkModes(c.nl, c.chat); err != nil {
			t.Errorf("-nl=%v -chat=%v отбито зря: %v", c.nl, c.chat, err)
		}
	}
}

// Чат-режим — такой же потребитель шлюза, как -nl: панель отладки обязана
// объяснять пустоту одинаково в обоих. Иначе игрок, запустивший -chat без
// -debug-llm, читает по Tab «запустите с -debug-llm», уже его указав.
func TestChatUsesModelsLikeNL(t *testing.T) {
	if !usesModels(false, true) {
		t.Error("-chat не признан режимом с моделями")
	}
	if usesModels(false, false) {
		t.Error("без флагов игра не должна поднимать шлюз")
	}
}
