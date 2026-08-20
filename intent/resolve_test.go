package intent

import "testing"

func TestResolveByNameHandlesRussianCases(t *testing.T) {
	scene := []Named{
		{"e_bern", "Берн, стражник"},
		{"e_nils", "Нильс, мальчишка-посыльный"},
	}
	cases := map[string]string{
		"Поздороваться с Берном": "e_bern",
		"поговорю с берном":      "e_bern",
		"спрошу Берна":           "e_bern",
		"обращаюсь к стражнику":  "e_bern",
		"поболтать с Нильсом":    "e_nils",
		"расспросить мальчишку":  "e_nils",
	}
	for text, want := range cases {
		got, ok := resolveByName(text, scene)
		if !ok || got != want {
			t.Errorf("%q -> (%q, %v), ожидалось %q", text, got, ok, want)
		}
	}
}

func TestResolveByNameRefusesAmbiguity(t *testing.T) {
	scene := []Named{{"e_bern", "Берн, стражник"}, {"e_nils", "Нильс, посыльный"}}
	if _, ok := resolveByName("поговорю с Берном и Нильсом", scene); ok {
		t.Error("двусмысленное упоминание разрешено в одну сущность")
	}
}

func TestResolveByNameFindsNothingWhenAbsent(t *testing.T) {
	scene := []Named{{"e_bern", "Берн, стражник"}}
	for _, text := range []string{"оглядываюсь", "поговорю с кузнецом", ""} {
		if id, ok := resolveByName(text, scene); ok {
			t.Errorf("%q разрешилось в %q, а такого в сцене нет", text, id)
		}
	}
}

func TestResolveByNameWorksForNodes(t *testing.T) {
	nodes := []Named{{"n_forge", "Кузница"}, {"n_guildhall", "Контора гильдии"}}
	got, ok := resolveByName("пойду в кузницу", nodes)
	if !ok || got != "n_forge" {
		t.Errorf("узел %q %v, ожидался n_forge", got, ok)
	}
}
