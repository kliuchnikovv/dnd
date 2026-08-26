package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Проба приземляется прозой, а не отказом. До этой ветки тот же ввод приходил
// игроку как «так не получится: словарь такого не покрывает» — служебный язык
// про устройство игры вместо отклика мира (ADR-0003, T1).
func TestProbeLandsAsProseNotRefusal(t *testing.T) {
	g := renderGame(t)
	fi := &fakeInterp{probe: "принюхивается к воздуху за бочками"}
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("принюхиваюсь за бочками\nquit\n"), &out).WithInterpreter(fi)
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "так не получится") || strings.Contains(got, "нельзя:") {
		t.Errorf("проба пришла отказом:\n%s", got)
	}
	if !strings.Contains(got, probeFallback) {
		t.Errorf("отклика на пробу нет:\n%s", got)
	}
}

// Чистая проба хода не тратит: наблюдение бесплатно, как look. Иначе
// исследование стоило бы часов, а часы — это давление дела.
func TestProbeCostsNoTime(t *testing.T) {
	g := renderGame(t)
	before := g.C.Snapshot()
	fi := &fakeInterp{probe: "ковыряет щель в настиле"}
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("ковыряю щель\nquit\n"), &out).WithInterpreter(fi)
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	after := g.C.Snapshot()
	for i := range before {
		if before[i].Filled != after[i].Filled {
			t.Errorf("проба тикнула часы %q: %d → %d",
				before[i].ID, before[i].Filled, after[i].Filled)
		}
	}
}
