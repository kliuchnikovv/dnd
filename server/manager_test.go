package server

import "testing"

func TestManagerCreateAndGet(t *testing.T) {
	m := NewManager(casesRoot)
	id, err := m.Create("harbour", 1)
	if err != nil {
		t.Fatalf("создание: %v", err)
	}
	rt, ok := m.Get(id)
	if !ok {
		t.Fatalf("сессия %q не найдена", id)
	}
	if rt.game == nil {
		t.Fatalf("сессия %q без игры", id)
	}
	if rt.caseID != "harbour" {
		t.Fatalf("caseID %q, ждали harbour", rt.caseID)
	}
}

// Две сессии одного дела на одном seed изолированы: разные chat_id и разные
// объекты игры. Ход в одной не должен трогать другую.
func TestManagerSessionsAreIsolated(t *testing.T) {
	m := NewManager(casesRoot)
	a, err := m.Create("harbour", 1)
	if err != nil {
		t.Fatalf("первая: %v", err)
	}
	b, err := m.Create("harbour", 1)
	if err != nil {
		t.Fatalf("вторая: %v", err)
	}
	if a == b {
		t.Fatalf("одинаковый chat_id у двух сессий: %q", a)
	}
	ra, _ := m.Get(a)
	rb, _ := m.Get(b)
	if ra.game == rb.game {
		t.Fatalf("две сессии делят один объект игры")
	}
}

// Неизвестное дело — ошибка, а не пустой chat_id.
func TestManagerUnknownCase(t *testing.T) {
	m := NewManager(casesRoot)
	if _, err := m.Create("нет-такого", 1); err == nil {
		t.Fatalf("ждали ошибку на неизвестном деле")
	}
}

// Get по неизвестному chat_id не находит и не паникует.
func TestManagerGetMissing(t *testing.T) {
	m := NewManager(casesRoot)
	if _, ok := m.Get("нет-такого-chat-id"); ok {
		t.Fatalf("нашли несуществующую сессию")
	}
}
