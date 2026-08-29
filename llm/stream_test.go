package llm

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// drain собирает поток до EOF и возвращает склеенный текст и число дельт.
func drain(t *testing.T, s Stream) (string, int) {
	t.Helper()
	var b strings.Builder
	n := 0
	for {
		d, err := s.Recv()
		b.WriteString(d.Text)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		n++
	}
	return b.String(), n
}

// Дельты склеиваются в тот же текст, что вернул бы Complete, а Response()
// проставлен шлюзом (модель, провайдер, стоимость).
func TestStreamDeltasReassemble(t *testing.T) {
	g, _ := gw(t, Caps{})
	st, err := g.Stream(context.Background(), Request{Role: RoleNarrator, Input: "опиши причал"})
	if err != nil {
		t.Fatal(err)
	}
	text, n := drain(t, st)
	if n < 2 {
		t.Fatalf("ждали несколько дельт, пришло %d", n)
	}
	want := "[fake/narrator] опиши причал"
	if text != want {
		t.Fatalf("склейка дельт %q, ждали %q", text, want)
	}
	resp := st.Response()
	if resp.Provider != "fake" || resp.Model != "claude-sonnet-5" {
		t.Fatalf("финал не проставлен: %+v", resp)
	}
	if resp.CostMicro <= 0 {
		t.Fatalf("стоимость не посчитана: %d", resp.CostMicro)
	}
}

// Расход снимается один раз — по завершении потока.
func TestStreamRecordsSpendOnce(t *testing.T) {
	g, _ := gw(t, Caps{})
	st, err := g.Stream(context.Background(), Request{Role: RoleNarrator, Input: "сцена"})
	if err != nil {
		t.Fatal(err)
	}
	// До вычитывания расхода ещё нет: usage известен только в конце.
	if g.Stats().CallsByRole[RoleNarrator] != 0 {
		t.Fatalf("расход снят до завершения потока")
	}
	drain(t, st)
	if got := g.Stats().CallsByRole[RoleNarrator]; got != 1 {
		t.Fatalf("вызовов нарратора %d, ждали 1", got)
	}
	if g.Stats().SpentMicro <= 0 {
		t.Fatalf("расход не учтён после завершения потока")
	}
}

// Потолок проверяется ДО провайдера и на потоковом пути: исчерпав бюджет
// пользователя одним потоком, второй отвергаем на Admit.
func TestStreamBudgetAdmitBlocks(t *testing.T) {
	g, _ := gw(t, Caps{PerUserDailyMicro: 1})
	req := Request{Role: RoleNarrator, Input: "длинная сцена на много токенов", UserID: "u1"}
	st, err := g.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("первый поток отвергнут: %v", err)
	}
	drain(t, st) // расход снимается здесь
	if _, err := g.Stream(context.Background(), req); !errors.Is(err, ErrUserBudget) {
		t.Fatalf("второй поток дал %v, ждали ErrUserBudget", err)
	}
}

// completeOnly — провайдер без потоковой генерации: у шлюза для него фолбэк
// через Complete.
type completeOnly struct{ inner *Fake }

func (c completeOnly) Name() string       { return "complete-only" }
func (c completeOnly) StrictOutput() bool  { return true }
func (c completeOnly) Complete(ctx context.Context, model string, r Request) (Response, error) {
	return c.inner.Complete(ctx, model, r)
}

// Провайдер без Stream обслуживается фолбэком: дельты всё равно склеиваются,
// расход учтён.
func TestStreamFallbackForCompleteOnlyProvider(t *testing.T) {
	p := completeOnly{inner: NewFake("fake", true)}
	r := NewRouter().Route(RoleNarrator, Target{p, "claude-sonnet-5"})
	g := NewGateway(r, NewLedger(Caps{}, WithClock(fixedClock("2026-08-18"))))

	st, err := g.Stream(context.Background(), Request{Role: RoleNarrator, Input: "сцена"})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := drain(t, st)
	if !strings.Contains(text, "сцена") {
		t.Fatalf("фолбэк потерял текст: %q", text)
	}
	if g.Stats().CallsByRole[RoleNarrator] != 1 {
		t.Fatalf("расход фолбэка не учтён")
	}
}

// Запрос со схемой не стримится: структурный вывод обязан прийти целиком, даже
// если провайдер умеет поток. Наблюдаем это по одному кадру вместо многих.
func TestStreamSchemaTakesFallback(t *testing.T) {
	g, _ := gw(t, Caps{})
	st, err := g.Stream(context.Background(), Request{
		Role:   RoleNarrator,
		Input:  "многословный ответ на схему",
		Schema: `{"properties":{"x":{"type":"string"}},"required":["x"]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, n := drain(t, st)
	if n != 1 {
		t.Fatalf("схема пришла %d дельтами, ждали один кадр (фолбэк)", n)
	}
}
