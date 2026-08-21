package main

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

func TestKnownModelNeedsNoPriceFlags(t *testing.T) {
	if err := ensurePrice("claude-opus-5", 0, 0); err != nil {
		t.Errorf("известная модель потребовала цену: %v", err)
	}
}

// Отказ должен наступать до первого вызова и называть, что делать. Узнать про
// неизвестную цену из ошибки на пятнадцатой минуте — значит потерять сессию.
func TestUnknownModelRefusesEarlyAndNamesTheFlags(t *testing.T) {
	err := ensurePrice("vendor/model-без-цены", 0, 0)
	if err == nil {
		t.Fatal("модель без цены принята")
	}
	for _, want := range []string{"-price-in", "-price-out", "vendor/model-без-цены"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("в ошибке нет %q: %v", want, err)
		}
	}
}

func TestGivenPricesAreRegistered(t *testing.T) {
	if err := ensurePrice("vendor/цена-руками", 3, 15); err != nil {
		t.Fatalf("цена не принята: %v", err)
	}
	if !llm.HasPrice("vendor/цена-руками") {
		t.Error("цена не зарегистрирована в шлюзе")
	}
	got, err := llm.CostMicro("vendor/цена-руками", llm.Usage{InputTokens: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if got != 3_000_000 {
		t.Errorf("миллион входных токенов стоит %d мкд, ожидалось 3000000", got)
	}
}
