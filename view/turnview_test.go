package view

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestTurnViewRoundTrips — вид переживает JSON без потерь. Это и есть весь
// смысл дескрипторов: та же структура станет телом ответа сервера, и если она
// не переживает сериализацию побайтово по значению, контракт врёт клиенту.
func TestTurnViewRoundTrips(t *testing.T) {
	max := 5
	orig := TurnView{
		Version: 1,
		Scene:   Scene{Node: "n_quay", Title: "Причал", ArtRef: "art_quay"},
		Narration: []Block{
			{Kind: "gm", Text: "Дождь бьёт по настилу.", Streaming: true},
			{Kind: "npc", Text: "Что вам надобно?", Speaker: &Who{ID: "e_bern", Name: "Берн"}},
		},
		Resolution: &Resolution{
			Terms:   []Term{{Label: "расследование", Value: 2}},
			Target:  12,
			Margin:  3,
			Outcome: Outcome{Label: "УСПЕХ", Tier: "success"},
		},
		Meters: []Meter{
			{Label: "раны", Kind: "harm", Value: 1, Max: &max, Surface: true},
			{Label: "grit", Kind: "grit", Value: 2, Surface: false},
		},
		Options: []Option{
			{ID: "o1", Label: "осмотреть причал", Token: "tok_examine", Check: "расследование"},
			{ID: "o2", Label: "«Что здесь было?»", Token: "tok_ask", Check: "общение", Reply: true},
		},
		Objective: &Panel{
			Kind: "deduction", Title: "Досье", Surface: false,
			Sections: []Section{{
				Label: "Факты",
				Items: []PanelItem{{ID: "f_rope", Label: "перерезанный трос", Sub: "Берн · 0.667", Token: "tok_f_rope"}},
				Slots: []Slot{{Name: "who", Label: "Кто", Options: []PanelItem{{ID: "f_bern", Label: "Берн"}}}},
			}},
		},
		Map: &MapView{
			ArtRef: "art_map",
			Nodes:  []MapNode{{ID: "n_quay", Name: "Причал", Reachable: true, Known: true, X: 1, Y: 2}},
		},
		Participants: []Who{{ID: "e_bern", Name: "Берн", AvatarRef: "av_bern", Disposition: -1}},
		Ended:        &Ending{Kind: "solved", Text: "Дело раскрыто."},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back TurnView
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(orig, back) {
		t.Errorf("round-trip разошёлся:\n orig=%+v\n back=%+v", orig, back)
	}
}

// TestOptionalFieldsAreOmitted — пустой вид не тащит по проводу ни резолюции,
// ни карты, ни панели, ни развязки. Это surfacing на уровне сериализации: чего
// нет, того клиент и не увидит в теле ответа.
func TestOptionalFieldsAreOmitted(t *testing.T) {
	data, err := json.Marshal(TurnView{Version: 1, Scene: Scene{Node: "n_quay"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{`"resolution"`, `"map"`, `"objective"`, `"ended"`} {
		if strings.Contains(s, key) {
			t.Errorf("пустой вид тащит %s: %s", key, s)
		}
	}
}
