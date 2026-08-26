package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// probeGame — узел с двумя целями: за телом стоит авторский гейт, бухты каната
// инертны. Ровно та развилка, которую разбирает маршрут пробы.
func probeGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	db.Entities["e_body"] = store.Entity{ID: "e_body", Name: "Тело Халдена",
		Kind: store.EntityThing, Node: "n_quay"}
	db.Entities["e_bern"] = store.Entity{ID: "e_bern", Name: "Берн, стражник",
		Kind: store.EntityNPC, Node: "n_quay"}
	db.Props["n_quay"] = []store.SceneProp{
		{ID: "p_coils", Node: "n_quay", Name: "Бухты каната", Kind: "clutter"},
	}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_ligature"] = store.Fact{ID: "f_ligature", Key: "борозда от шнура"}
	db.Facts["f_watch"] = store.Fact{ID: "f_watch", Key: "ночной обход"}
	db.Holders["f_ligature"] = []store.FactHolder{{
		FactID: "f_ligature", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "easy"},
	}}
	db.Holders["f_watch"] = []store.FactHolder{{
		FactID: "f_watch", HolderID: "e_bern", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}
	db.Clocks["c_suspicion"] = &store.Clock{ID: "c_suspicion", Segments: 6, TickPolicy: "on_cost"}
	return NewGame(Config{
		DB: db, Rules: fixedRules{OutcomeSuccess}, Dice: nilDice{},
		Truth:   accusation.NewTruth("bern", "cord", "night", "debt"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

// Проба, назвавшая авторскую цель, становится обычным ходом — и глагол берётся
// ИЗ ГЕЙТА автора, а не угадывается по тексту. Иначе «щупаю шею» молча
// подменялось бы осмотром, и игра делала бы не то, что заявлено.
func TestProbeOnAuthoredTargetTakesTheGateVerb(t *testing.T) {
	g := probeGame()
	in, ok := g.MatchProbe("щупаю шею у тела")
	if !ok {
		t.Fatal("проба не легла на авторскую цель")
	}
	if in.Verb != "examine" || in.Args.Target != "e_body" {
		t.Fatalf("проба разобралась как %s → %q", in.Verb, in.Args.Target)
	}
	if res := g.Apply(in); res.Refused {
		t.Fatalf("собранный пробой ход отклонён ядром: %s", res.Refusal)
	}
	if !g.K.Knows("f_ligature") {
		t.Error("авторский факт не открылся: свободный текст до него не доехал")
	}
}

// Инертная деталь места держателя не несёт: проба на неё — только
// повествование. Ядро об этом честно говорит «не совпало», а не подсовывает
// осмотр, которого игрок не заявлял.
func TestProbeOnInertPropDoesNotMatch(t *testing.T) {
	g := probeGame()
	if in, ok := g.MatchProbe("пинаю бухты каната"); ok {
		t.Errorf("инертная деталь дала ход %s → %q", in.Verb, in.Args.Target)
	}
}

// Проба, назвавшая человека, к авторскому не маршрутизируется. К людям
// обращаются, а не пробуют: превратить фразу в допрос значило бы выбрать за
// игрока механику, которой он не заявлял. Речь и обращение разбирает парсер.
func TestProbeDoesNotRouteThroughPeople(t *testing.T) {
	g := probeGame()
	if in, ok := g.MatchProbe("заглядываю Берну в глаза"); ok {
		t.Errorf("проба уехала в допрос: %s → %q", in.Verb, in.Args.Target)
	}
}

// Двусмысленность догадкой не разрешается — то же правило, что у разбора
// команд: два совпадения означают, что цель не названа.
func TestAmbiguousProbeDoesNotMatch(t *testing.T) {
	g := probeGame()
	g.DB.Entities["e_body2"] = store.Entity{ID: "e_body2", Name: "Тело собаки",
		Kind: store.EntityThing, Node: "n_quay"}
	if _, ok := g.MatchProbe("осматриваю тело"); ok {
		t.Error("двусмысленная проба разрешена догадкой")
	}
}

// Уже известный факт цель не открывает второй раз, и проба это видит: иначе
// игрок мог бы бесконечно «пробовать» одну деталь, получая ход за ходом
// авторский резолв на пустом месте.
func TestProbeDoesNotMatchWhenNothingIsLeftBehindTheTarget(t *testing.T) {
	g := probeGame()
	in, ok := g.MatchProbe("щупаю шею у тела")
	if !ok {
		t.Fatal("проба не легла на авторскую цель")
	}
	g.Apply(in)
	if _, ok := g.MatchProbe("щупаю шею у тела"); ok {
		t.Error("исчерпанная цель по-прежнему притягивает пробу")
	}
}

// Проба ничего не меняет, пока её не применили: MatchProbe только читает.
// Третьей двери в состояние не появляется (ADR-0001).
func TestMatchProbeChangesNothing(t *testing.T) {
	g := probeGame()
	before := g.C.Snapshot()
	g.MatchProbe("щупаю шею у тела")
	if g.K.Knows("f_ligature") {
		t.Error("MatchProbe выдал факт — он обязан только читать")
	}
	after := g.C.Snapshot()
	for i := range before {
		if before[i].Filled != after[i].Filled {
			t.Errorf("MatchProbe тикнул часы %q", before[i].ID)
		}
	}
}
