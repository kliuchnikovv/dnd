package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
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

// probeGame — узел, где за телом стоит авторский гейт на неизвестный факт, а
// ворох сетей инертен. minimal.json для этого не годится: его единственный факт
// известен со старта, и цель исчерпана до первого хода.
func probeGame(t *testing.T) *core.Game {
	t.Helper()
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Name: "Пристань"}
	db.Entities["e_body"] = store.Entity{ID: "e_body", Name: "Тело Халдена",
		Kind: store.EntityThing, Node: "n_quay"}
	db.Props["n_quay"] = []store.SceneProp{
		{ID: "p_nets", Node: "n_quay", Name: "Ворох сетей", Kind: "clutter"},
	}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_ligature"] = store.Fact{ID: "f_ligature", Key: "След шнура на шее"}
	db.Holders["f_ligature"] = []store.FactHolder{{
		FactID: "f_ligature", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "easy"},
	}}
	db.Clocks["c_suspicion"] = &store.Clock{ID: "c_suspicion", Segments: 6, TickPolicy: "on_cost"}
	return core.NewGame(core.Config{
		DB: db, Rules: threshold.New(), Dice: dice.NewSource(1).Stream("resolve"),
		Truth:   accusation.NewTruth("toke", "cord", "night", "debt"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

// Проба, назвавшая авторскую цель, идёт обычным ходом: журнал, ядро, факт.
// Так авторский контент становится достижим словами, а не только командой.
func TestProbeOnAuthoredTargetBecomesATurn(t *testing.T) {
	g := probeGame(t)
	fi := &fakeInterp{probe: "щупает шею у тела"}
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("щупаю шею у тела\nquit\n"), &out).WithInterpreter(fi)
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if !g.K.Knows("f_ligature") {
		t.Errorf("проба не доехала до авторского факта:\n%s", out.String())
	}
	if strings.Contains(out.String(), probeFallback) {
		t.Errorf("совпавшая проба ушла в повествование:\n%s", out.String())
	}
}

// Несовпавшая проба состояния не трогает вовсе — ни фактов, ни часов. Канон
// меняется только через валидируемый путь ядра.
func TestUnmatchedProbeLeavesCanonAlone(t *testing.T) {
	g := probeGame(t)
	fi := &fakeInterp{probe: "ковыряет ворох сетей"}
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("ковыряю сети\nquit\n"), &out).WithInterpreter(fi)
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if g.K.Knows("f_ligature") {
		t.Error("чистая проба выдала факт")
	}
	if !strings.Contains(out.String(), probeFallback) {
		t.Errorf("отклика на пробу нет:\n%s", out.String())
	}
}

// Отклик на чистую пробу — заказ на прозу Мастера, а не готовая строка. Без
// Мастера печатается рамка: игра без моделей обязана работать как работала.
func TestPureProbeAsksTheMasterForProse(t *testing.T) {
	g := probeGame(t)
	var got Prose
	r := Render{Narrate: func(p Prose) string {
		got = p
		return "Сети пахнут тиной; ничего, кроме тины."
	}}
	fi := &fakeInterp{probe: "ковыряет ворох сетей"}
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("ковыряю сети\nquit\n"), &out).WithInterpreter(fi)
	s.r = r
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if got.Kind != ProseProbe {
		t.Errorf("заказ на прозу пришёл видом %q", got.Kind)
	}
	if got.Probe != "ковыряет ворох сетей" {
		t.Errorf("проба не доехала до Мастера: %q", got.Probe)
	}
	if !strings.Contains(out.String(), "ничего, кроме тины") {
		t.Errorf("проза Мастера не напечатана:\n%s", out.String())
	}
}

// Мастеру нельзя выдать то, чего парти не знает. Проверяется весь заказ, а не
// одна рамка: утечка ходит тем же путём, что проза (ADR-0003, T2).
func TestProbeProseNamesNoUnknownFact(t *testing.T) {
	g := probeGame(t)
	var got Prose
	s := NewSession(g, strings.NewReader("ковыряю сети\nquit\n"), &bytes.Buffer{}).
		WithInterpreter(&fakeInterp{probe: "ковыряет ворох сетей"})
	s.r = Render{Narrate: func(p Prose) string { got = p; return "" }}
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	material := got.Frame + " " + got.Probe + " " +
		strings.Join(got.Scene, " ") + " " + strings.Join(got.Outcome, " ")
	for id, f := range g.DB.Facts {
		if g.K.Knows(id) {
			continue
		}
		if f.Key != "" && strings.Contains(material, f.Key) {
			t.Errorf("в заказ на прозу пробы попал неизвестный факт %q", f.Key)
		}
	}
}

// В чат-режиме проба идёт тем же маршрутом: сначала авторское, потом
// повествование. Отдельной ветки у чата быть не должно — два разбора одной
// фразы разошлись бы молча.
func TestChatProbeRoutesLikeTheOtherMode(t *testing.T) {
	g := probeGame(t)
	fc := &fakeChat{probe: "щупает шею у тела"}
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("щупаю шею у тела\nquit\n"), &out).WithChat(fc)
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if !g.K.Knows("f_ligature") {
		t.Errorf("проба чат-режима не доехала до авторского факта:\n%s", out.String())
	}
}

// Реплика Мастера пробу не предваряет. Подводка предваряет ДЕЙСТВИЕ, а у пробы
// отклик и есть весь её текст: показать оба значило бы описать одно событие
// дважды, причём вторым — тем же голосом.
func TestChatReplyDoesNotPrecedeAProbe(t *testing.T) {
	g := probeGame(t)
	fc := &fakeChat{probe: "ковыряет ворох сетей", reply: "Вы тянетесь к сетям."}
	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("ковыряю сети\nquit\n"), &out).WithChat(fc)
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if strings.Contains(out.String(), "Вы тянетесь к сетям") {
		t.Errorf("реплика предварила пробу — одно событие описано дважды:\n%s", out.String())
	}
	if !strings.Contains(out.String(), probeFallback) {
		t.Errorf("отклика на пробу нет:\n%s", out.String())
	}
}

// Проба чат-режима оставляет строку аудита: модель по недоверенному вводу
// высказалась, и инъекция живёт ровно здесь (ADR-0002, ADR-0003 T4).
func TestChatProbeLeavesAnAuditTrail(t *testing.T) {
	g := probeGame(t)
	fc := &fakeChat{probe: "ковыряет ворох сетей"}
	s := NewSession(g, strings.NewReader("ковыряю сети\nquit\n"), &bytes.Buffer{}).
		WithChat(fc).WithJournal(NewJournal(g.DB, "s-chat-probe", "snap", 1))
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	var found bool
	for _, e := range g.DB.Audit {
		if strings.Contains(e.LLMProposal, "ковыряет ворох сетей") {
			found = true
		}
	}
	if !found {
		t.Errorf("проба чат-режима не оставила следа в аудите: %+v", g.DB.Audit)
	}
	if len(g.DB.CommandLog) != 0 {
		t.Errorf("чистая проба попала в журнал команд: %+v", g.DB.CommandLog)
	}
}
