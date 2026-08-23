package llm

import "testing"

// AllRoles перечисляет роли руками, а руки забывают: роль чат-режима была
// объявлена и вызывалась, но осталась без маршрута, и прогон на живой модели
// упал в ErrNoProvider на первом же ходу. Проводка теперь проверяется по
// AllRoles исчерпывающе — значит сам AllRoles обязан быть полным, иначе
// проверка проверяет неполный список и молчит.
//
// Список сверяется с ОБЪЯВЛЕНИЯМИ в role.go, а не со вторым списком в тесте:
// второй список отстал бы точно так же.
func TestAllRolesListsEveryDeclaredRole(t *testing.T) {
	declared := declaredRoles(t)
	if len(declared) == 0 {
		t.Fatal("в role.go не нашлось ни одной константы роли — сломан разбор")
	}

	listed := map[Role]bool{}
	for _, r := range AllRoles() {
		if listed[r] {
			t.Errorf("роль %q перечислена в AllRoles дважды", r)
		}
		listed[r] = true
	}

	for name, role := range declared {
		if !listed[role] {
			t.Errorf("константа %s (%q) объявлена, но в AllRoles её нет: "+
				"проводка не проверит роль, которой список не знает", name, role)
		}
	}
	for role := range listed {
		if _, ok := valueSet(declared)[role]; !ok {
			t.Errorf("AllRoles называет %q, которой в role.go нет", role)
		}
	}
}

// declaredRoles — константы типа Role из role.go: имя к значению. Разбор
// общий с реестром капабилити (capability_test.go): второй разборщик отстал
// бы ровно так же, как отстал бы второй список ролей.
func declaredRoles(t *testing.T) map[string]Role {
	t.Helper()
	out := map[string]Role{}
	for name, value := range declaredConsts(t, "role.go", "Role") {
		out[name] = Role(value)
	}
	return out
}

func valueSet(m map[string]Role) map[Role]struct{} {
	out := make(map[Role]struct{}, len(m))
	for _, v := range m {
		out[v] = struct{}{}
	}
	return out
}
