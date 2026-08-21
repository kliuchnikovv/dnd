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
	want := `{"fact_id":"f_ligature","holder_id":"e_body","holder_kind":"",` +
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

// --- Requirement: порог предпосылок гейта ---

func requirementFrom(t *testing.T, raw string) *Requirement {
	t.Helper()
	var h FactHolder
	if err := json.Unmarshal([]byte(`{"gate":{"requires":`+raw+`}}`), &h); err != nil {
		t.Fatalf("разбор %s: %v", raw, err)
	}
	return h.Gate.Requires
}

func TestRequirementAcceptsFlatArrayAsAll(t *testing.T) {
	// Исторический вид: плоский массив означает «нужны все».
	r := requirementFrom(t, `["f_a","f_b"]`)
	if r == nil {
		t.Fatal("плоский массив не разобрался")
	}
	if len(r.Of) != 2 || r.N != 2 {
		t.Errorf("Of=%v N=%d, ожидалось 2 факта и N=2", r.Of, r.N)
	}
}

func TestRequirementObjectWithoutNMeansAll(t *testing.T) {
	r := requirementFrom(t, `{"of":["f_a","f_b","f_c"]}`)
	if r.N != 3 {
		t.Errorf("N=%d, ожидалось 3 (все)", r.N)
	}
}

func TestRequirementObjectWithNIsThreshold(t *testing.T) {
	r := requirementFrom(t, `{"of":["f_a","f_b","f_c"],"n":2}`)
	if len(r.Of) != 3 || r.N != 2 {
		t.Errorf("Of=%v N=%d, ожидалось 3 факта и N=2", r.Of, r.N)
	}
}

func TestRequirementAbsentIsNil(t *testing.T) {
	var h FactHolder
	if err := json.Unmarshal([]byte(`{"gate":{"verbs":["examine"]}}`), &h); err != nil {
		t.Fatal(err)
	}
	if h.Gate.Requires != nil {
		t.Errorf("отсутствующие предпосылки дали %+v, ожидался nil", h.Gate.Requires)
	}
}

func TestRequirementSatisfiedCountsMatches(t *testing.T) {
	r := requirementFrom(t, `{"of":["f_a","f_b","f_c"],"n":2}`)
	known := map[FactID]bool{}
	knows := func(f FactID) bool { return known[f] }

	if r.Satisfied(knows) {
		t.Error("порог 2 выполнен при нуле известных фактов")
	}
	known["f_a"] = true
	if r.Satisfied(knows) {
		t.Error("порог 2 выполнен при одном известном факте")
	}
	known["f_c"] = true
	if !r.Satisfied(knows) {
		t.Error("порог 2 не выполнен при двух известных фактах")
	}
	known["f_b"] = true
	if !r.Satisfied(knows) {
		t.Error("порог 2 перестал выполняться при трёх известных фактах")
	}
}

func TestRequirementNOfOneIsOr(t *testing.T) {
	r := requirementFrom(t, `{"of":["f_a","f_b","f_c"],"n":1}`)
	known := map[FactID]bool{"f_b": true}
	if !r.Satisfied(func(f FactID) bool { return known[f] }) {
		t.Error("n=1 не сработал как OR")
	}
}

func TestNilRequirementIsSatisfied(t *testing.T) {
	var r *Requirement
	if !r.Satisfied(func(FactID) bool { return false }) {
		t.Error("отсутствие предпосылок должно считаться выполненным")
	}
}
