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
