package e2e

import (
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/store"
)

var caseFiles = map[string]string{
	"harbour":     "../cases/harbour/case.json",
	"forte_merlo": "../cases/forte_merlo/case.json",
}

// Камуфляж имеет нижний порог, а не только верхний. Узел, где интерактивных
// целей ровно столько, сколько холдеров, — это карта решения: игрок видит
// четыре пункта и понимает, что всё дело в них.
func TestEveryNodeHasAtLeastAsManyPropsAsHolders(t *testing.T) {
	for name, path := range caseFiles {
		t.Run(name, func(t *testing.T) {
			cfg, err := cases.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			// Считаются РАЗЛИЧНЫЕ держатели, а не строки: камуфляж меряется
			// тем, что игрок видит в списке, а сущность с шестью фактами
			// печатается одной строкой.
			seen := map[store.EntityID]bool{}
			holders := map[store.NodeID]int{}
			for _, hs := range cfg.DB.Holders {
				for _, h := range hs {
					if seen[h.HolderID] {
						continue
					}
					seen[h.HolderID] = true
					holders[cfg.DB.Entities[h.HolderID].Node]++
				}
			}
			for node := range cfg.DB.Locations {
				props := len(cfg.DB.Props[node])
				if props < holders[node] {
					t.Errorf("узел %s: %d пропов при %d холдерах — список выдаёт граф",
						node, props, holders[node])
				}
				if props < 3 {
					t.Errorf("узел %s: %d пропов, бюджет камуфляжа 3–5", node, props)
				}
			}
		})
	}
}
