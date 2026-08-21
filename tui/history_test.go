package tui

import "testing"

// Стрелка вверх поднимает последнее введённое, вниз возвращает обратно.
func TestHistoryWalksBackAndForth(t *testing.T) {
	var h History
	h.Add("осмотреться")
	h.Add("поговорить с Берном")

	if got, ok := h.Prev(); !ok || got != "поговорить с Берном" {
		t.Fatalf("первый шаг назад дал %q (%v)", got, ok)
	}
	if got, ok := h.Prev(); !ok || got != "осмотреться" {
		t.Fatalf("второй шаг назад дал %q (%v)", got, ok)
	}
	if _, ok := h.Prev(); ok {
		t.Error("шаг назад за начало истории удался")
	}
	if got, ok := h.Next(); !ok || got != "поговорить с Берном" {
		t.Errorf("шаг вперёд дал %q (%v)", got, ok)
	}
}

// Пустой ввод и повтор подряд историю не засоряют: иначе стрелка вверх
// перелистывает одно и то же.
func TestHistorySkipsEmptyAndRepeats(t *testing.T) {
	var h History
	h.Add("look")
	h.Add("look")
	h.Add("   ")
	h.Add("")

	if got, _ := h.Prev(); got != "look" {
		t.Errorf("последнее в истории %q", got)
	}
	if _, ok := h.Prev(); ok {
		t.Error("в истории больше одной записи — повторы и пустое не отсеяны")
	}
}

// После отправки история читается с конца: игрок ждёт последнюю фразу первым
// же нажатием, а не продолжения прошлой прогулки по списку.
func TestHistoryResetsPositionAfterAdd(t *testing.T) {
	var h History
	h.Add("first")
	h.Add("second")
	h.Prev()
	h.Prev()
	h.Add("third")
	if got, _ := h.Prev(); got != "third" {
		t.Errorf("после ввода стрелка вверх дала %q", got)
	}
}
