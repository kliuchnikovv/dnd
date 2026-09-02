package accusation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func testTruth() Truth {
	return NewTruth("toke", "seal_cord", "night_before_tide", "audit_shortfall")
}

func TestTruthNeverPrints(t *testing.T) {
	tr := testTruth()
	for _, s := range []string{
		fmt.Sprint(tr),
		fmt.Sprintf("%v", tr),
		fmt.Sprintf("%s", tr),
		tr.String(),
	} {
		if s != "<redacted>" {
			t.Errorf("truth просочился в вывод: %q", s)
		}
		for _, leak := range []string{"toke", "seal_cord", "night_before_tide", "audit_shortfall"} {
			if strings.Contains(s, leak) {
				t.Errorf("вывод содержит %q", leak)
			}
		}
	}
}

func TestTruthRefusesToMarshal(t *testing.T) {
	if _, err := json.Marshal(testTruth()); err == nil {
		t.Fatal("truth сериализовался в JSON — утечка через любой дамп состояния")
	}
}

func TestCheckAcceptsOnlyAllFourSlots(t *testing.T) {
	tr := testTruth()
	full := Form{Who: "toke", How: "seal_cord", When: "night_before_tide", Why: "audit_shortfall"}
	if !tr.Check(full) {
		t.Fatal("верное обвинение отвергнуто")
	}
	// Три из четырёх — всё ещё неверно.
	three := full
	three.Why = "old_grudge"
	if tr.Check(three) {
		t.Error("обвинение с одной ошибкой принято")
	}
	if tr.Check(Form{}) {
		t.Error("пустое обвинение принято")
	}
}

func TestCheckReturnsOnlyBool(t *testing.T) {
	// Тип возврата — единственный булев: интерфейс не может проговориться,
	// в каком слоте ошибка. Тест фиксирует это на уровне сигнатуры.
	tr := testTruth()
	var _ func(Form) bool = tr.Check
}
