package forte_merlo_test

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/store"
)

func TestForteMerloLoadsAndValidates(t *testing.T) {
	if _, err := cases.Load("case.json"); err != nil {
		t.Fatalf("дело не проходит валидацию:\n%v", err)
	}
}

func TestFlavourIsRealProseNotStubs(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Flavour) < 60 {
		t.Errorf("текстов всего %d — дело недописано", len(cfg.Flavour))
	}
	for key, text := range cfg.Flavour {
		if len([]rune(text)) < 20 {
			t.Errorf("текст %q слишком короток: %q", key, text)
		}
		for _, stub := range []string{"TODO", "TBD", "stub", "lorem", "заглушка"} {
			if strings.Contains(strings.ToLower(text), stub) {
				t.Errorf("текст %q — заглушка: %q", key, text)
			}
		}
	}
}

// Порог 2-из-3 — та фича, ради которой расширяли движок. Дело обязано её нести.
func TestHowSlotUsesTwoOfThreeThreshold(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, h := range cfg.DB.Holders["f_strangled_and_staged"] {
		req := h.Gate.Requires
		if req == nil {
			continue
		}
		if len(req.Of) == 3 && req.N == 2 {
			found = true
		}
	}
	if !found {
		t.Error("держатель f_strangled_and_staged не использует порог 2 из 3")
	}
}

func TestTruthFactsHaveIndependentSources(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	// Факты, закрывающие слоты обвинения, должны собираться из независимых
	// источников — иначе корроборация не даст порога 0.8.
	for _, f := range []store.FactID{"f_drug_program", "f_emilien_at_dawn"} {
		var srcs []store.EntityID
		for _, h := range cfg.DB.Holders[f] {
			srcs = append(srcs, h.HolderID)
		}
		if len(srcs) < 3 {
			t.Errorf("у %s всего %d источников: %v", f, len(srcs), srcs)
			continue
		}
		for i := range srcs {
			for j := i + 1; j < len(srcs); j++ {
				if cfg.DB.Related(srcs[i], srcs[j]) {
					t.Errorf("%s: источники %s и %s связаны — не сложатся",
						f, srcs[i], srcs[j])
				}
			}
		}
	}
}
