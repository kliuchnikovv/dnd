package store

import (
	"encoding/json"
	"testing"
)

func TestFactHolderJSONTags(t *testing.T) {
	h := FactHolder{
		FactID:    "f_ligature",
		HolderID:  "e_body",
		Gate:      Gate{Verbs: []string{"examine"}, Threshold: "normal"},
		Mandatory: true,
		Latent:    false,
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	want := `{"fact_id":"f_ligature","holder_id":"e_body",` +
		`"gate":{"verbs":["examine"],"threshold":"normal","requires":null},` +
		`"mandatory":true,"latent":false}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestKnowledgeCarriesSource(t *testing.T) {
	// Ключ party_knowledge — пара (fact_id, learned_from_entity).
	// Два свидетельства об одном факте — две разные строки.
	a := Knowledge{FactID: "f_shortfall", LearnedFrom: "e_toke", Confidence: 0.5, LearnedAt: 1}
	b := Knowledge{FactID: "f_shortfall", LearnedFrom: "e_sigrid", Confidence: 0.5, LearnedAt: 2}
	if a.Key() == b.Key() {
		t.Fatalf("два источника одного факта дали один ключ: %v", a.Key())
	}
}
