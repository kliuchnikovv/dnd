package master

import (
	"context"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

func TestOptionsAreVoicedInOrder(t *testing.T) {
	m, f := masterWith(t, "Где вы были в ту ночь?\nВзгляните — предписание.\nосмотреть бочки")
	got, err := m.Options(context.Background(), World{Setting: "Гавань"}, []Option{
		{Text: "расспросить Берн про след шнура", Reply: true},
		{Text: "предъявить Предписание магистрата — Берн", Reply: true},
		{Text: "осмотреть Штабель бочек", Reply: false},
	}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("строк %d, вариантов было 3: %v", len(got), got)
	}
	if !strings.Contains(got[0], "ту ночь") {
		t.Errorf("первая строка не о первом варианте: %q", got[0])
	}
	if f.Calls()[0].Role != llm.RoleOptions {
		t.Errorf("озвучка вызвана чужой ролью: %q", f.Calls()[0].Role)
	}
	if f.Calls()[0].Tier != llm.TierCheap {
		t.Errorf("озвучка ушла тиром %q — это работа дешёвой модели", f.Calls()[0].Tier)
	}
}

// Слова применяются целиком либо не применяются вовсе. Частичное применение —
// худший из отказов: строка съезжает на соседний интент, и игрок, выбравший
// «поблагодарить», угрожает.
func TestPartialVoicingIsRefused(t *testing.T) {
	m, _ := masterWith(t, "Где вы были в ту ночь?\nВзгляните — предписание.")
	_, err := m.Options(context.Background(), World{}, []Option{
		{Text: "первый", Reply: true},
		{Text: "второй", Reply: true},
		{Text: "третий", Reply: false},
	}, llm.Request{})
	if err == nil {
		t.Fatal("две строки на три варианта приняты — строка съедет на соседний интент")
	}
}

// Пустой набор модель не беспокоит: вызов без нужды это деньги за шум.
func TestEmptySetIsNotSentToTheModel(t *testing.T) {
	m, f := masterWith(t, "что угодно")
	got, err := m.Options(context.Background(), World{}, nil, llm.Request{})
	if err != nil || got != nil {
		t.Errorf("пустой набор дал %v, %v", got, err)
	}
	if len(f.Calls()) != 0 {
		t.Error("на пустой набор потрачен вызов модели")
	}
}

// Мастеру видно, где реплика, а где действие: реплику он пишет от первого лица
// словами игрока, действие остаётся действием.
func TestPromptSeparatesRepliesFromActions(t *testing.T) {
	m, f := masterWith(t, "а\nб")
	m.Options(context.Background(), World{}, []Option{
		{Text: "расспросить Берн", Reply: true},
		{Text: "осмотреть бочки", Reply: false},
	}, llm.Request{})
	in := f.Calls()[0].Input
	if !strings.Contains(in, "РЕПЛИКА") || !strings.Contains(in, "ДЕЙСТВИЕ") {
		t.Errorf("Мастер не различает реплику и действие:\n%s", in)
	}
}
