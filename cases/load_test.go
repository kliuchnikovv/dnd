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
