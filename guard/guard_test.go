package guard

import (
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

// Проверка идёт основным тиром, и это не расточительность, а измерение:
// дешёвый судья пропускает одну утечку из десяти (corpus_test.go), а утечка
// бьёт в решаемость дела.
func TestGuardGoesMainTier(t *testing.T) {
	if got := New(nil).tier(); got != llm.TierMain {
		t.Errorf("страж пошёл тиром %q — дешёвый пропускает утечки", got)
	}
}
