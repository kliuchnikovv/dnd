package llm

import (
	"context"
	"errors"
	"testing"
)

// Кому нужен строгий провайдер — записано здесь руками нарочно. Вывод из
// реестра сверять с самим реестром было бы тавтологией: таблица держит
// намерение, и сдвиг полномочий обязан её уронить, а не молча пройти.
var wantStrict = map[Role]bool{
	RoleIntentParser: true,  // предлагает команду
	RoleChatMaster:   true,  // предлагает команду
	RoleNarrator:     true,  // предлагает канон-ambient
	RoleWorldsmith:   true,  // предлагает канон и мутацию мира
	RoleActor:        true,  // предлагает место, о котором рассказал
	RoleCanonGuard:   false, // вердикт-совет
	RoleModeration:   false, // вердикт-совет
	RoleOptions:      false, // её выход не доходит до состояния
}

func TestRequiresStrictOutputFollowsRegistry(t *testing.T) {
	for _, role := range AllRoles() {
		want, ok := wantStrict[role]
		if !ok {
			t.Errorf("роль %q появилась, а таблица строгости о ней не знает", role)
			continue
		}
		if got := RequiresStrictOutput(role); got != want {
			t.Errorf("RequiresStrictOutput(%q) = %v, ожидалось %v", role, got, want)
		}
		// MutatesState остаётся синонимом: у него есть вызывающие, и
		// разъехавшиеся ответы означали бы две правды вместо реестра.
		if got := role.MutatesState(); got != want {
			t.Errorf("%q.MutatesState() = %v, ожидалось %v", role, got, want)
		}
	}
}

// Роутер обязан фильтровать по реестру, а не по своему представлению о ролях:
// иначе объявленное полномочие и допущенный провайдер разъезжаются.
func TestRouterFiltersByRegistry(t *testing.T) {
	loose := Target{NewFake("loose", false), "claude-haiku-4-5"}
	strict := Target{NewFake("strict", true), "claude-haiku-4-5"}
	for _, role := range AllRoles() {
		r := NewRouter().Route(role, loose)
		if got := len(r.Chain(role)); (got == 0) != RequiresStrictOutput(role) {
			t.Errorf("роль %q: нестрогих целей в цепочке %d при строгости %v",
				role, got, RequiresStrictOutput(role))
		}
		if got := len(NewRouter().Route(role, strict).Chain(role)); got != 1 {
			t.Errorf("роль %q: строгая цель отфильтрована", role)
		}
	}
}

// Дешёвый тир — та же цепочка по тем же правилам: экономия не повод пускать
// предложение канона через провайдера без гарантии схемы.
func TestCheapTierFiltersByRegistryToo(t *testing.T) {
	loose := Target{NewFake("loose", false), "claude-haiku-4-5"}
	strict := Target{NewFake("strict", true), "claude-sonnet-5"}
	r := NewRouter().Route(RoleNarrator, strict).RouteCheap(RoleNarrator, loose)
	if got := len(r.ChainFor(RoleNarrator, TierCheap)); got != 0 {
		t.Errorf("дешёвая нестрогая цель прошла к нарратору: целей %d", got)
	}
}

// Отфильтрованная дочиста роль — это «настроено небезопасно», а не «не
// настроено»: шлюз обязан различать их, иначе причина отказа теряется.
func TestGatewayRefusesUnsafeProviderForProposingRoles(t *testing.T) {
	for _, role := range AllRoles() {
		loose := NewFake("loose", false)
		g := NewGateway(
			NewRouter().Route(role, Target{loose, "claude-haiku-4-5"}),
			NewLedger(Caps{}, WithClock(fixedClock("2026-08-18"))),
		)
		_, err := g.Do(context.Background(), Request{Role: role, Input: "текст"})
		switch {
		case RequiresStrictOutput(role):
			if !errors.Is(err, ErrSchemaUnsafe) {
				t.Errorf("роль %q: ошибка %v, ожидалась ErrSchemaUnsafe", role, err)
			}
			if len(loose.Calls()) != 0 {
				t.Errorf("роль %q: провайдер без гарантии схемы был вызван", role)
			}
		default:
			if err != nil {
				t.Errorf("роль %q отвергнута: %v", role, err)
			}
		}
	}
}
