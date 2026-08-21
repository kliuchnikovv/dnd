package core

import (
	"strconv"
	"strings"
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

// Актёр без памяти не помнит, что игрок представился ходом раньше. Порядок
// реплик несущий: разговор читается сверху вниз, иначе модель видит кашу.
func TestRememberKeepsOrder(t *testing.T) {
	d := NewDossiers(dossierDB(), "party")

	d.Remember("e_bern", "здравствуйте", "и вам", 1)
	d.Remember("e_bern", "я из магистрата", "вижу", 2)

	got := d.Recent("e_bern")
	if len(got) != 2 {
		t.Fatalf("вспомнилось %d реплик, ждали 2", len(got))
	}
	if got[0].Player != "здравствуйте" || got[0].Reply != "и вам" || got[0].Turn != 1 {
		t.Errorf("первая реплика не та: %+v", got[0])
	}
	if got[1].Player != "я из магистрата" {
		t.Errorf("порядок сбился: %+v", got[1])
	}
}

// Память растит промпт, поэтому у неё есть потолок: старое сворачивается в
// впечатления, а не копится до бесконечности.
func TestRememberFoldsOldestIntoSummary(t *testing.T) {
	db := dossierDB()
	d := NewDossiers(db, "party")

	for i := 1; i <= historyCap+3; i++ {
		d.Remember("e_bern", "вопрос "+itoa(i), "ответ "+itoa(i), i)
	}

	if got := len(d.Recent("e_bern")); got != historyCap {
		t.Errorf("в истории %d реплик, потолок %d", got, historyCap)
	}
	if first := d.Recent("e_bern")[0]; first.Turn != 4 {
		t.Errorf("обрезали не с начала: первый ход %d, ждали 4", first.Turn)
	}
	summary := db.DossierFor("e_bern", "party").Summary
	if !strings.Contains(summary, "вопрос 1") {
		t.Errorf("вытесненное не свернулось в впечатления: %q", summary)
	}
	if n := len([]rune(summary)); n > summaryCap {
		t.Errorf("впечатления разрослись до %d рун, потолок %d", n, summaryCap)
	}
}

// Впечатления тоже не растут без предела: очень долгий разговор вытесняет
// самое старое, а не переполняет промпт.
func TestSummaryStaysBounded(t *testing.T) {
	db := dossierDB()
	d := NewDossiers(db, "party")

	for i := 1; i <= historyCap*20; i++ {
		d.Remember("e_bern", "вопрос "+itoa(i), "ответ "+itoa(i), i)
	}

	summary := db.DossierFor("e_bern", "party").Summary
	if n := len([]rune(summary)); n > summaryCap {
		t.Errorf("впечатления %d рун, потолок %d", n, summaryCap)
	}
	if !strings.Contains(summary, "вопрос "+itoa(historyCap*20-historyCap)) {
		t.Errorf("свежее вытесненное потерялось: %q", summary)
	}
}

// Разговор — отношенческий слой: что человек рассказал одной парти, второй он
// не рассказывал.
func TestHistoryIsPerParty(t *testing.T) {
	db := dossierDB()
	a, b := NewDossiers(db, "party-a"), NewDossiers(db, "party-b")

	a.Remember("e_bern", "здравствуйте", "и вам", 1)

	if len(a.Recent("e_bern")) != 1 {
		t.Error("первая парти забыла свой разговор")
	}
	if got := b.Recent("e_bern"); len(got) != 0 {
		t.Errorf("вторая парти видит чужой разговор: %+v", got)
	}
	if world, _ := db.WorldDossier("e_bern"); len(world.History) != 0 {
		t.Error("разговор протёк в мировой слой")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// Быт — мировой слой: он у человека один и общий для всех парти. Держать его
// ещё и в отношенческом значило бы иметь два места правды.
func TestLifeComesFromWorldLayer(t *testing.T) {
	db := dossierDB()
	world, _ := db.WorldDossier("e_bern")
	world.Life = "смена с рассвета, ворчит на сырость"

	a, b := NewDossiers(db, "party-a"), NewDossiers(db, "party-b")
	if a.Life("e_bern") != "смена с рассвета, ворчит на сырость" {
		t.Errorf("быт не доехал: %q", a.Life("e_bern"))
	}
	if a.Life("e_bern") != b.Life("e_bern") {
		t.Error("быт разошёлся между парти — он объективен")
	}
	if got := a.Life("e_никого"); got != "" {
		t.Errorf("быт взялся из ниоткуда: %q", got)
	}
}
