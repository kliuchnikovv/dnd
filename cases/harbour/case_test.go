package harbour_test

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/store"
)

func TestHarbourCaseLoadsAndValidates(t *testing.T) {
	if _, err := cases.Load("case.json"); err != nil {
		t.Fatalf("дело не проходит валидацию:\n%v", err)
	}
}

func TestFlavourIsRealProseNotStubs(t *testing.T) {
	// Заглушки дадут ложный негатив на главном вопросе M1a: с «TODO» вместо
	// текста расследование не может ощущаться игрой ни при какой механике.
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Flavour) < 30 {
		t.Errorf("текстов всего %d — дело недописано", len(cfg.Flavour))
	}
	for key, text := range cfg.Flavour {
		if len([]rune(text)) < 20 {
			t.Errorf("текст %q слишком короток: %q", key, text)
		}
		for _, stub := range []string{"TODO", "TBD", "заглушка", "lorem"} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(stub)) {
				t.Errorf("текст %q — заглушка: %q", key, text)
			}
		}
	}
}

func TestEveryTruthFactHasThreeIndependentSources(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"f_seal_cord", "f_shortfall", "f_toke_at_quay", "f_tide_night"} {
		var holders []string
		for _, h := range cfg.DB.Holders[storeFact(f)] {
			holders = append(holders, string(h.HolderID))
		}
		if len(holders) < 3 {
			t.Errorf("у %s всего %d источников: %v", f, len(holders), holders)
		}
	}
}

func storeFact(s string) store.FactID { return store.FactID(s) }
