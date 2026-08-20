package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func dossierDB() *store.DB {
	db := store.NewDB()
	db.Entities["e_bern"] = store.Entity{ID: "e_bern", Kind: store.EntityNPC, Voice: "сухой"}
	world := db.DossierFor("e_bern", "")
	world.Voice = "служебный"
	world.KnowsAbout = []store.FactID{"f_secret"}
	world.TalksAbout = []string{"приливы"}
	return db
}

// Мировой слой объективен, отношенческий — про конкретную парти. Без этого
// разделения кооп склеит знание разных парти.
func TestRelationalLayerIsPerParty(t *testing.T) {
	db := dossierDB()
	a, b := NewDossiers(db, "party-a"), NewDossiers(db, "party-b")

	a.Adjust("e_bern", -3)
	if got := a.Disposition("e_bern"); got != -3 {
		t.Errorf("расположение у первой парти %d", got)
	}
	if got := b.Disposition("e_bern"); got != 0 {
		t.Errorf("расположение второй парти %d — слои склеились", got)
	}
	// Мировой слой общий для обеих.
	if a.Voice("e_bern") != b.Voice("e_bern") {
		t.Error("голос разошёлся между парти")
	}
}

func TestRelationalVoiceOverridesWorld(t *testing.T) {
	db := dossierDB()
	d := NewDossiers(db, "party")
	if got := d.Voice("e_bern"); got != "служебный" {
		t.Errorf("голос %q, ожидался из дневника", got)
	}
	db.DossierFor("e_bern", "party").Voice = "сорванный"
	if got := d.Voice("e_bern"); got != "сорванный" {
		t.Errorf("голос %q — отношенческий слой должен перебивать мировой", got)
	}
}

func TestVoiceFallsBackToEntity(t *testing.T) {
	db := store.NewDB()
	db.Entities["e_x"] = store.Entity{ID: "e_x", Kind: store.EntityNPC, Voice: "из сущности"}
	if got := NewDossiers(db, "party").Voice("e_x"); got != "из сущности" {
		t.Errorf("голос %q", got)
	}
}

// Знать не значит рассказать: условия выдачи задаёт gate, а Knows отвечает
// только на механический вопрос.
func TestKnowsIsMechanical(t *testing.T) {
	d := NewDossiers(dossierDB(), "party")
	if !d.Knows("e_bern", "f_secret") {
		t.Error("не знает того, что записано в дневнике")
	}
	if d.Knows("e_bern", "f_другое") {
		t.Error("знает то, чего в дневнике нет")
	}
}

func TestOpenThreadsAreClosable(t *testing.T) {
	db := dossierDB()
	d := NewDossiers(db, "party")
	db.DossierFor("e_bern", "party").OpenThreads = []string{"долг", "обещание"}

	if got := len(d.OpenThreads("e_bern")); got != 2 {
		t.Fatalf("незакрытых дел %d", got)
	}
	if !d.CloseThread("e_bern", "долг") {
		t.Error("дело не закрылось")
	}
	if got := d.OpenThreads("e_bern"); len(got) != 1 || got[0] != "обещание" {
		t.Errorf("осталось %v", got)
	}
	if d.CloseThread("e_bern", "нет такого") {
		t.Error("закрыто несуществующее дело")
	}
}

func TestViewMergesBothLayers(t *testing.T) {
	db := dossierDB()
	db.DossierFor("e_bern", "party").TalksAbout = []string{"ночная смена"}
	view := NewDossiers(db, "party").View("e_bern")
	if len(view.TalksAbout) != 2 {
		t.Errorf("тем %d, ожидалось 2 — слои не слились: %v", len(view.TalksAbout), view.TalksAbout)
	}
}
