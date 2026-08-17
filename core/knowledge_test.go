package core

import (
	"math"
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func knowledgeDB() *store.DB {
	db := store.NewDB()
	db.Facts["f_shortfall"] = store.Fact{ID: "f_shortfall", Key: "shortfall"}
	for _, id := range []store.EntityID{"e_toke", "e_sigrid", "e_bern", "e_nils"} {
		db.Entities[id] = store.Entity{ID: id, Kind: store.EntityNPC}
	}
	// Сигрид и Нильс общаются — за два независимых источника не считаются.
	db.Relations = append(db.Relations, store.Relation{From: "e_sigrid", To: "e_nils", Kind: "aunt"})
	return db
}

func TestTwoSourcesOneFactAreTwoRows(t *testing.T) {
	k := NewKnowledge(knowledgeDB())
	if !k.Learn("f_shortfall", "e_toke") {
		t.Fatal("первый источник не записан")
	}
	if !k.Learn("f_shortfall", "e_bern") {
		t.Fatal("второй источник не записан")
	}
	if got := len(k.Sources("f_shortfall")); got != 2 {
		t.Errorf("источников %d, ожидалось 2", got)
	}
	// Повторное свидетельство того же источника новой строки не даёт.
	if k.Learn("f_shortfall", "e_toke") {
		t.Error("дубликат источника записан как новый")
	}
}

func TestConfidenceCombinesIndependentSources(t *testing.T) {
	k := NewKnowledge(knowledgeDB())
	k.Learn("f_shortfall", "e_toke")
	assertClose(t, k.Confidence("f_shortfall"), 0.5)
	k.Learn("f_shortfall", "e_bern")
	assertClose(t, k.Confidence("f_shortfall"), 0.75)
	k.Learn("f_shortfall", "e_sigrid")
	assertClose(t, k.Confidence("f_shortfall"), 0.875)
	if !k.Corroborated("f_shortfall") {
		t.Error("три независимых источника не дали корроборации")
	}
}

func TestRelatedSourcesDoNotStack(t *testing.T) {
	k := NewKnowledge(knowledgeDB())
	k.Learn("f_shortfall", "e_toke")
	k.Learn("f_shortfall", "e_sigrid")
	k.Learn("f_shortfall", "e_nils") // связан с Сигрид — не засчитывается
	assertClose(t, k.Confidence("f_shortfall"), 0.75)
	if k.Corroborated("f_shortfall") {
		t.Error("связанные источники дали корроборацию")
	}
}

func TestTopicBankHoldsOnlyKnownFacts(t *testing.T) {
	k := NewKnowledge(knowledgeDB())
	if len(k.TopicBank()) != 0 {
		t.Fatal("банк тем непуст на старте — спрашивать не о чем")
	}
	k.Learn("f_shortfall", "e_toke")
	bank := k.TopicBank()
	if len(bank) != 1 || bank[0] != "f_shortfall" {
		t.Errorf("банк тем = %v, ожидалось [f_shortfall]", bank)
	}
}

func assertClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("confidence = %v, ожидалось %v", got, want)
	}
}
