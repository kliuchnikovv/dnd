package main

import (
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

// Отчёт покрывает каждую зароученную роль. Свой список уже разошёлся с
// проводкой один раз: RoleChatMaster в нём не было, и прогон под -chat выглядел
// бесплатным — вызовы шли, а строки про них не было. Роль, за которую платят и
// о которой молчат, замечают по счёту, а не по логам.
func TestReportCoversEveryRoutedRole(t *testing.T) {
	reported := map[llm.Role]bool{}
	for _, r := range reportedRoles() {
		if reported[r] {
			t.Errorf("роль %s в отчёте дважды", r)
		}
		reported[r] = true
	}
	for _, r := range appRoles {
		if !reported[r.Role] {
			t.Errorf("роль %s зароучена, но в отчёт не попала", r.Role)
		}
	}
}

// Порядок отчёта — порядок объявления. Отчёт читают глазами, и переставленные
// от прогона к прогону строки сравнивать нельзя.
func TestReportedRolesKeepDeclarationOrder(t *testing.T) {
	got := reportedRoles()
	var want []llm.Role
	seen := map[llm.Role]bool{}
	for _, r := range appRoles {
		if !seen[r.Role] {
			seen[r.Role] = true
			want = append(want, r.Role)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("ролей %d, ожидалось %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("на месте %d роль %s, ожидалась %s", i, got[i], want[i])
		}
	}
}
