package main

import (
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

// Роль, которой игра пользуется, но которую забыли зароутить, падает в
// ErrNoProvider — а откат надстройки молчит. Так проза Мастера не работала
// целую фазу: код был написан, вызов не доходил до модели, а игрок видел
// просто авторский текст.
//
// Тест закрывает весь класс: список ролей объявлен рядом с проводкой, и
// каждая из них обязана иметь цель на том тире, которым спрашивает.
func TestEveryRoleTheGameUsesIsRouted(t *testing.T) {
	main := llm.Target{Provider: llm.NewFake("main", true), Model: "claude-sonnet-5"}

	// Без дешёвой модели всё идёт основной: молча дешеветь за счёт качества
	// нельзя, но и остаться без цели роль не имеет права.
	only := buildRouter(main, llm.Target{})
	for _, use := range appRoles {
		if len(only.ChainFor(use.Role, use.Tier)) == 0 {
			t.Errorf("роль %q на тире %q осталась без цели", use.Role, use.Tier)
		}
	}

	cheap := llm.Target{Provider: main.Provider, Model: "claude-haiku-4-5"}
	both := buildRouter(main, cheap)
	for _, use := range appRoles {
		chain := both.ChainFor(use.Role, use.Tier)
		if len(chain) == 0 {
			t.Fatalf("роль %q на тире %q осталась без цели", use.Role, use.Tier)
		}
		want := main.Model
		if use.Tier == llm.TierCheap {
			want = cheap.Model
		}
		if chain[0].Model != want {
			t.Errorf("роль %q на тире %q пошла в %q, ждали %q",
				use.Role, use.Tier, chain[0].Model, want)
		}
	}
}

// Проверка реплики дешёвого тира не просит: измерение показало, что дешёвый
// судья пропускает утечки. Значит и разводить её по тирам нечего — иначе в
// проводке остаётся мёртвая ветка, которая выглядит рабочей.
func TestGuardHasNoCheapRoute(t *testing.T) {
	for _, use := range appRoles {
		if use.Role == llm.RoleCanonGuard && use.Tier == llm.TierCheap {
			t.Error("проверка реплики объявлена дешёвой — она судит основным тиром")
		}
	}
}

// notUsed — роли, объявленные в llm, но игрой не вызываемые. Список
// исчерпывающий: всё, чего в нём нет, обязано быть в appRoles.
//
// Тест ниже нужен потому, что appRoles ведётся руками, и этого оказалось
// недостаточно: роль чат-режима была объявлена, вызывалась и НЕ была
// зароучена — прогон на живой модели упал в ErrNoProvider на первом же ходу,
// не сломав ни одного теста. Теперь новая роль обязана попасть либо в
// проводку, либо сюда — с причиной.
var notUsed = map[llm.Role]string{
	llm.RoleModeration: "модерация не подключена",
	llm.RoleWorldsmith: "генерации дел ещё нет",
}

func TestEveryDeclaredRoleIsEitherRoutedOrDeliberatelyUnused(t *testing.T) {
	used := map[llm.Role]bool{}
	for _, use := range appRoles {
		used[use.Role] = true
	}
	for _, role := range llm.AllRoles() {
		switch {
		case used[role] && notUsed[role] != "":
			t.Errorf("роль %q и в проводке, и в списке неиспользуемых", role)
		case !used[role] && notUsed[role] == "":
			t.Errorf("роль %q объявлена, но нигде не решено, вызывает её игра "+
				"или нет: добавьте её в appRoles либо в notUsed с причиной", role)
		}
	}
}
