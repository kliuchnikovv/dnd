package core_test

import (
	"go/build"
	"strings"
	"testing"
)

func TestCoreDoesNotImportRules(t *testing.T) {
	pkg, err := build.Default.Import("github.com/kliuchnikovv/dnd/core", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, "kliuchnikovv/dnd/rules/") {
			t.Errorf("core импортирует rules-конкретику: %s", imp)
		}
		// Примечание: сейчас core тянет core/scenarios/deduction/accusation
		// через тип Truth. Это techdebt: Truth-типы должны переехать в store
		// или core как нейтральные данные автора. До того — этот аспект
		// теста ослаблен намеренно (см. ledger, ruling у Task 4).
	}
}
