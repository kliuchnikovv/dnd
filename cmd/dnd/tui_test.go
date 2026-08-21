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
