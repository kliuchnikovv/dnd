package vignette

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/dice"
)

// Рекордер пишет валидный JSONL: шапка-ключ с правдой + строки ходов с
// input/ruling/revealed/master_output — материал для офлайн-оценки утечек.
func TestRecorderWritesHeaderAndTurns(t *testing.T) {
	var buf bytes.Buffer
	sc := guestScene()
	rec := NewRecorder(&buf, sc)

	j := KeywordJudge{}
	st := NewState(dice.Fixed(12))
	for _, in := range []string{"прислушиваюсь к двери", "жду"} {
		r := j.Rule(in, BuildJudgeView(sc, st))
		res := st.Adjudicate(sc, r)
		rec.Turn(in, r, res, "проза Мастера тут")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("ожидалось 3 строки JSONL (шапка + 2 хода), получено %d", len(lines))
	}
	var header map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("шапка не JSON: %v", err)
	}
	if header["type"] != "header" || header["truth"] != sc.Truth {
		t.Fatalf("шапка без ключа-правды: %v", header)
	}
	var turn map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &turn); err != nil {
		t.Fatalf("строка хода не JSON: %v", err)
	}
	if turn["type"] != "turn" || turn["master_output"] != "проза Мастера тут" {
		t.Fatalf("строка хода неполна: %v", turn)
	}
}
