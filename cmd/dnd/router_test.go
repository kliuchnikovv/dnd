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
