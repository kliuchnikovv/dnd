package cases

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadBuildsPlayableConfig(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка: %v", err)
	}
	if cfg.Start != "n_quay" || cfg.Actor != "pc" {
		t.Errorf("стартовое состояние: узел %q, актор %q", cfg.Start, cfg.Actor)
	}
	if len(cfg.DB.Locations) != 2 || len(cfg.DB.Entities) != 2 {
		t.Errorf("таблицы заполнены не полностью: %d локаций, %d сущностей",
			len(cfg.DB.Locations), len(cfg.DB.Entities))
	}
	if got := cfg.DB.Locations["n_quay"].Tags; len(got) != 1 || got[0] != "rain" {
		t.Errorf("теги локации = %v", got)
	}
	if len(cfg.DB.Holders["f_ligature"]) != 1 {
		t.Error("держатель факта не загружен")
	}
	if cfg.Flavour["look.n_quay"] == "" {
		t.Error("флейвор не загружен")
	}
	if len(cfg.Tokens) != 4 {
		t.Errorf("токенов обвинения %d, ожидалось 4", len(cfg.Tokens))
	}
}

func TestStartFactsArePrewritten(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DB.Knowledge) != 1 || cfg.DB.Knowledge[0].FactID != "f_ligature" {
		t.Errorf("стартовые факты не записаны: %v", cfg.DB.Knowledge)
	}
}

func TestSheetSurvivesAsRawBytes(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Attrs map[string]int `json:"attrs"`
	}
	if err := json.Unmarshal(cfg.DB.Characters["pc"].Sheet, &probe); err != nil {
		t.Fatalf("лист не разобрался: %v", err)
	}
	if probe.Attrs["mind"] != 3 {
		t.Errorf("mind = %d, ожидалось 3", probe.Attrs["mind"])
	}
}

func TestTruthNeverAppearsInConfigDump(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	// Сериализация конфига обязана падать, а не печатать правильный ответ.
	if b, err := json.Marshal(cfg.Truth); err == nil {
		t.Fatalf("truth сериализовался: %s", b)
	}
	if s := cfg.Truth.String(); strings.Contains(s, "toke") {
		t.Errorf("truth просочился: %q", s)
	}
}

func TestBrokenJSONReportsPath(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.json"); err == nil {
		t.Fatal("несуществующий файл загрузился")
	}
	if _, err := Parse([]byte("{не json")); err == nil {
		t.Fatal("битый JSON загрузился")
	}
}

// Кто держит факт, тот его знает. Выводится из fact_holders, а не пишется
// руками: расхождение проявилось бы только в разговоре.
func TestDossierKnowledgeDerivedFromHolders(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	world, ok := cfg.DB.WorldDossier("e_body")
	if !ok {
		t.Fatal("у держателя факта нет мирового дневника")
	}
	var found bool
	for _, f := range world.KnowsAbout {
		if f == "f_ligature" {
			found = true
		}
	}
	if !found {
		t.Errorf("держатель не знает своего факта: %v", world.KnowsAbout)
	}
}

// Затравка мира — авторская рамка, за которую можно цепляться, не выдумывая.
// Без неё персонажу нечего сказать о быте, и он либо молчит, либо сочиняет.
func TestSettingAndLifeReachTheGame(t *testing.T) {
	cfg, err := Load("harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(cfg.Setting) == "" {
		t.Error("сеттинг дела не доехал в конфиг")
	}
	world, ok := cfg.DB.WorldDossier("e_bern")
	if !ok {
		t.Fatal("у Берна нет мирового слоя — фикстура сломана")
	}
	if strings.TrimSpace(world.Life) == "" {
		t.Error("быт Берна не доехал в мировой слой")
	}
	// Быт объективен: он у человека один, а не свой для каждой парти.
	if rel := cfg.DB.Dossiers[dossierKeyFor("e_bern")]; rel != nil && rel.Life != "" {
		t.Error("быт продублирован в отношенческий слой — два места правды")
	}
}

// Оба поля необязательны: старое дело обязано грузиться без них.
func TestSettingAndLifeAreOptional(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatalf("дело без затравки не загрузилось: %v", err)
	}
	if cfg.Setting != "" {
		t.Errorf("сеттинг взялся из ниоткуда: %q", cfg.Setting)
	}
}

// Предметы дела и стартовый инвентарь: без них у детектива нет рычага,
// который в жанре базовый — «у меня бумага, ты обязан говорить».
func TestItemsAndStartInventoryLoad(t *testing.T) {
	cfg, err := Load("testdata/items.json")
	if err != nil {
		t.Fatal(err)
	}
	writ, ok := cfg.DB.Items["i_writ"]
	if !ok {
		t.Fatal("предмет не загрузился")
	}
	if writ.Kind != "credential" || writ.Name == "" {
		t.Errorf("предмет загрузился неполно: %+v", writ)
	}
	// CaseID проставляет загрузчик, как и факту: предмет принадлежит делу, а
	// не глобальному пространству.
	if writ.CaseID != cfg.CaseID {
		t.Errorf("дело предмета %q, а дело %q", writ.CaseID, cfg.CaseID)
	}
	if !cfg.DB.HasItem(defaultParty, "i_writ") {
		t.Error("стартовый инвентарь не доехал до парти")
	}
	if cfg.DB.HasItem(defaultParty, "i_lamp") {
		t.Error("в инвентаре предмет, которого не было в start_inventory")
	}
}

// Дело без предметов грузится как раньше: оба поля необязательны.
func TestItemsAreOptional(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatalf("дело без предметов не загрузилось: %v", err)
	}
	if len(cfg.DB.Items) != 0 || len(cfg.DB.Inventory) != 0 {
		t.Errorf("предметы взялись из ниоткуда: %+v", cfg.DB.Items)
	}
}

// Стартовый инвентарь не может назвать предмет, которого в деле нет: это
// опечатка автора, и её надо видеть на загрузке, а не в середине партии.
func TestStartInventoryRejectsUnknownItem(t *testing.T) {
	if _, err := Load("testdata/items_broken.json"); err == nil {
		t.Fatal("дело с несуществующим предметом в старт-инвентаре загрузилось")
	}
}

// Место, названное брифингом, обязано быть известно с начала: иначе игрок
// читает про склад в первой строке и не может туда пойти.
func TestStartPlacesReachTheGame(t *testing.T) {
	cfg, err := Load("harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, n := range cfg.StartPlaces {
		if n == "n_warehouse" {
			found = true
		}
	}
	if !found {
		t.Error("склад не объявлен известным с начала")
	}
}
