package store

import "testing"

func newTestDB() *DB {
	db := NewDB()
	db.Entities["e_ivar"] = Entity{ID: "e_ivar", Name: "Ивар", Kind: EntityNPC, Node: "n_forge"}
	db.Entities["e_toke"] = Entity{ID: "e_toke", Name: "Токе", Kind: EntityNPC, Node: "n_guildhall"}
	db.Entities["e_sigrid"] = Entity{ID: "e_sigrid", Name: "Сигрид", Kind: EntityNPC, Node: "n_forge"}
	db.Relations = append(db.Relations, Relation{From: "e_ivar", To: "e_sigrid", Kind: "married"})
	db.Locations["n_forge"] = Location{ID: "n_forge", Adjacent: []NodeID{"n_quay"}}
	db.Locations["n_quay"] = Location{ID: "n_quay", Adjacent: []NodeID{"n_forge"}}
	return db
}

func TestRelatedIsSymmetric(t *testing.T) {
	db := newTestDB()
	if !db.Related("e_ivar", "e_sigrid") {
		t.Error("ребро не найдено в прямом направлении")
	}
	if !db.Related("e_sigrid", "e_ivar") {
		t.Error("ребро не найдено в обратном направлении")
	}
	if db.Related("e_ivar", "e_toke") {
		t.Error("несвязанные сущности объявлены связанными")
	}
}

func TestEntitiesAtFiltersByNode(t *testing.T) {
	db := newTestDB()
	got := db.EntitiesAt("n_forge")
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 сущности в кузнице, получено %d", len(got))
	}
	// Порядок детерминирован — иначе вывод сцены пляшет от прогона к прогону.
	if got[0].ID != "e_ivar" || got[1].ID != "e_sigrid" {
		t.Errorf("порядок не детерминирован: %v, %v", got[0].ID, got[1].ID)
	}
}

func TestAdjacentRejectsUnlinkedNodes(t *testing.T) {
	db := newTestDB()
	if !db.Adjacent("n_forge", "n_quay") {
		t.Error("смежные узлы объявлены несмежными")
	}
	if db.Adjacent("n_forge", "n_guildhall") {
		t.Error("несмежные узлы объявлены смежными")
	}
}

// Инвентарь носит ПАРТИ, а не персонаж: в одиночной игре это одно и то же, но
// ключевать его парти с первого дня дешевле, чем мигрировать под кооп.
func TestInventoryIsPerParty(t *testing.T) {
	db := NewDB()
	db.Items["i_writ"] = Item{ID: "i_writ", Kind: "credential", Name: "Предписание"}

	db.AddItem("party-a", "i_writ")

	if !db.HasItem("party-a", "i_writ") {
		t.Error("парти не несёт то, что ей дали")
	}
	if db.HasItem("party-b", "i_writ") {
		t.Error("предмет одной парти виден другой")
	}
	if db.HasItem("party-a", "i_нет_такого") {
		t.Error("парти несёт предмет, которого ей не давали")
	}
}

// Добавление идемпотентно: предмет либо есть, либо нет, счётчика у него нет.
func TestAddItemIsIdempotent(t *testing.T) {
	db := NewDB()
	db.AddItem("party", "i_writ")
	db.AddItem("party", "i_writ")
	if got := len(db.Inventory); got != 1 {
		t.Errorf("строк инвентаря %d после двух добавлений", got)
	}
}
