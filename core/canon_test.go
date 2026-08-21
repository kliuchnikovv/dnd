package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func canonGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Name: "Пристань"}
	return NewGame(Config{DB: db, CaseID: "harbour", Start: "n_quay"})
}

// Канон мира держит слово: спросили второй раз — вернулось то же, а не новая
// выдумка. Без этого «расширение мира» становится генератором противоречий.
func TestCanonIsFirstWins(t *testing.T) {
	g := canonGame()

	if got := g.CanonPut("ключи от весовой", "у смотрителя весов", 3); got != "у смотрителя весов" {
		t.Errorf("первая канонизация вернула %q", got)
	}
	if got := g.CanonPut("ключи от весовой", "у начальника стражи", 7); got != "у смотрителя весов" {
		t.Errorf("вторая канонизация переписала канон: %q", got)
	}
	got, ok := g.CanonGet("ключи от весовой")
	if !ok || got != "у смотрителя весов" {
		t.Errorf("канон читается как %q (%v)", got, ok)
	}
}

// Тема нормализуется: разный регистр и лишние пробелы — это один и тот же
// вопрос, и второй ответ на него был бы вторым каноном.
func TestCanonTopicIsNormalised(t *testing.T) {
	g := canonGame()
	g.CanonPut("Ключи от Весовой", "у смотрителя весов", 1)
	if got, ok := g.CanonGet("  ключи   от весовой "); !ok || got != "у смотрителя весов" {
		t.Errorf("нормализация темы не работает: %q (%v)", got, ok)
	}
}

// Пустая тема или пустой ответ каноном не становятся: канон из ничего — это
// дыра, в которую потом уедет любая выдумка.
func TestCanonRejectsEmpty(t *testing.T) {
	g := canonGame()
	g.CanonPut("", "что-то", 1)
	g.CanonPut("тема", "   ", 1)
	if got := g.Canon(); len(got) != 0 {
		t.Errorf("пустое ушло в канон: %+v", got)
	}
}

// Канон читается в стабильном порядке: промпт обязан собираться одинаково.
func TestCanonReadsInStableOrder(t *testing.T) {
	g := canonGame()
	g.CanonPut("яблоки", "с материка", 1)
	g.CanonPut("аптека", "нет, только травница на площади", 2)

	got := g.Canon()
	if len(got) != 2 || got[0].Topic != "аптека" || got[1].Topic != "яблоки" {
		t.Errorf("порядок канона нестабилен: %+v", got)
	}
}

// Канон ключуется делом: деталь, решённую в одном деле, нельзя молча
// применять к другому.
func TestCanonIsPerCase(t *testing.T) {
	db := store.NewDB()
	a := NewGame(Config{DB: db, CaseID: "harbour"})
	b := NewGame(Config{DB: db, CaseID: "forte_merlo"})

	a.CanonPut("аптека", "нет такой", 1)
	if _, ok := b.CanonGet("аптека"); ok {
		t.Error("канон одного дела виден в другом")
	}
}
