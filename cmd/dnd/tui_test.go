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
