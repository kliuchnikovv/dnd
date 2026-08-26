package harbour_test

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/naming"
	"github.com/kliuchnikovv/dnd/store"
)

func TestHarbourCaseLoadsAndValidates(t *testing.T) {
	if _, err := cases.Load("case.json"); err != nil {
		t.Fatalf("дело не проходит валидацию:\n%v", err)
	}
}

func TestFlavourIsRealProseNotStubs(t *testing.T) {
	// Заглушки дадут ложный негатив на главном вопросе M1a: с «TODO» вместо
	// текста расследование не может ощущаться игрой ни при какой механике.
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Flavour) < 30 {
		t.Errorf("текстов всего %d — дело недописано", len(cfg.Flavour))
	}
	for key, text := range cfg.Flavour {
		if len([]rune(text)) < 20 {
			t.Errorf("текст %q слишком короток: %q", key, text)
		}
		for _, stub := range []string{"TODO", "TBD", "заглушка", "lorem"} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(stub)) {
				t.Errorf("текст %q — заглушка: %q", key, text)
			}
		}
	}
}

func TestEveryTruthFactHasThreeIndependentSources(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"f_seal_cord", "f_shortfall", "f_toke_at_quay", "f_tide_night"} {
		var holders []string
		for _, h := range cfg.DB.Holders[storeFact(f)] {
			holders = append(holders, string(h.HolderID))
		}
		if len(holders) < 3 {
			t.Errorf("у %s всего %d источников: %v", f, len(holders), holders)
		}
	}
}

func storeFact(s string) store.FactID { return store.FactID(s) }

// Персонажу нужно, о чём говорить. Без этого он либо молчит, либо начинает
// выдумывать — и то и другое ломает разговор.
func TestEveryTalkingNPCHasSomethingToSay(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	for id, e := range cfg.DB.Entities {
		if e.Kind != store.EntityNPC || e.Voice == "" {
			continue
		}
		world, ok := cfg.DB.WorldDossier(id)
		if !ok || len(world.TalksAbout) == 0 {
			t.Errorf("%s (%s) не о чем говорить — дневник пуст", id, e.Name)
			continue
		}
		for _, topic := range world.TalksAbout {
			if len([]rune(topic)) < 20 {
				t.Errorf("%s: тема слишком коротка: %q", id, topic)
			}
		}
		rel := cfg.DB.DossierFor(id, "party")
		if len(rel.OpenThreads) == 0 {
			t.Errorf("%s: нет незакрытых дел — разговору некуда продолжаться", id)
		}
	}
}

// Каждое место из start_places названо брифингом.
//
// Спека знания мест объявляет start_places «местами, о которых игрок знает из
// брифинга». Обещание было нарушено молча: в списке лежали кузница и контора
// гильдии, а брифинг называл один склад — и живой прогон получил вариант
// «перейти: Кузница», не имея ни малейшего повода знать, что кузница есть.
//
// Тест здесь, у дела, а не в общих инвариантах: связь прозы с узлом
// машиночитаемой не бывает, и общее правило пришлось бы либо ослабить до
// бесполезного, либо навязать другим делам чужую манеру брифинга. Это обещание
// ЭТОГО автора, и держать его надо рядом с его текстом.
//
// Сверка идёт тем же naming.Mentions, которым игра разбирает имена во вводе
// игрока: падеж ей не помеха, а если автор назвал место так, что инструмент
// игры его не узнаёт, то и игрок, скорее всего, тоже.
func TestBriefingNamesEveryStartPlace(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	brief := strings.ToLower(cfg.Briefing)
	for _, n := range cfg.StartPlaces {
		name := cfg.DB.Locations[n].Name
		if name == "" {
			t.Errorf("место %s из start_places не существует", n)
			continue
		}
		if !naming.Mentions(brief, name) {
			t.Errorf("брифинг не называет %s (%q) — игроку неоткуда о нём знать",
				n, name)
		}
	}
}

// Таверна брифингом не названа и в start_places не лежит — и это НЕ упущение.
// Про неё узнают в игре: место, о котором рассказал персонаж, — единственный
// путь знания, который проверяет саму механику рассказа. Пропади он, и
// импровизация персонажей снова станет украшением.
func TestTavernIsLearnedInPlayNotBriefed(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range cfg.StartPlaces {
		if n == "n_tavern" {
			t.Fatal("таверна попала в start_places — единственный путь знания через рассказ закрыт")
		}
	}
	if cfg.DB.Locations["n_tavern"].Name == "" {
		t.Fatal("таверны в деле нет — тест сторожит несуществующее")
	}
}
