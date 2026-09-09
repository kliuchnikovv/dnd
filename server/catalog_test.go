package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestGetCasesReturnsCatalog(t *testing.T) {
	cat := &CaseCatalog{
		entries: []CaseSummary{
			{ID: "c_harbour", Name: "Пристань", Rules: "threshold", Scenario: "deduction"},
			{ID: "c_lighthouse", Name: "Ночной маяк", Rules: "dnd5e", Scenario: "adventure"},
		},
	}
	req := httptest.NewRequest("GET", "/cases", nil)
	rec := httptest.NewRecorder()
	cat.HandleList(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code: %d", rec.Code)
	}
	var got []CaseSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "c_harbour" {
		t.Fatalf("got: %+v", got)
	}
}

// TestLoadCatalogReadsRealCases — регрессия: все реальные дела репозитория
// (deduction и adventure) грузятся каталогом без ошибок, включая
// специфичное для lighthouse поле "rules", которого нет в схеме cases.File.
func TestLoadCatalogReadsRealCases(t *testing.T) {
	cat, err := LoadCatalog(casesRoot)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	want := map[string]struct{ rules, kind string }{
		"harbour":      {"threshold", CaseKindAdventure},
		"forte_merlo":  {"threshold", CaseKindAdventure},
		"c_lighthouse": {"dnd5e", CaseKindAdventure},
		"nightguest":   {VignetteRulesKind, CaseKindVignette},
	}
	got := map[string]CaseSummary{}
	for _, e := range cat.entries {
		got[e.ID] = e
	}
	for id, w := range want {
		e, ok := got[id]
		if !ok {
			t.Fatalf("дело %q не попало в каталог: %+v", id, cat.entries)
		}
		if e.Rules != w.rules {
			t.Errorf("%s: rules = %q, ожидалось %q", id, e.Rules, w.rules)
		}
		if e.Kind != w.kind {
			t.Errorf("%s: kind = %q, ожидалось %q", id, e.Kind, w.kind)
		}
	}
}
