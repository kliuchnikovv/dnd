package core

import (
	"encoding/json"
	"testing"
)

// Имена полей интента — контракт журнала действий (ADR-0002): правда реплея
// это структурный интент, и переименование поля в коде не имеет праваménять
// смысл уже записанных сессий.
func TestIntentJSONTagsAreTheJournalContract(t *testing.T) {
	b, err := json.Marshal(Intent{
		Verb:  "question",
		Actor: "c_pc",
		Args:  Args{Target: "e_toke", Topic: "f_ligature"},
		Push:  true,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"verb":"question","actor":"c_pc",` +
		`"args":{"target":"e_toke","topic":"f_ligature"},"push":true}`
	if got := string(b); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// Незаполненные аргументы в записи не появляются: журнал сессии читают глазами
// при разборе бага, и восемь пустых полей на каждый ход этому не помогают.
func TestEmptyArgsStayOutOfTheRecord(t *testing.T) {
	b, err := json.Marshal(Intent{Verb: "look"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(b), `{"verb":"look","args":{}}`; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// Версия правил существует и непуста: реплей сверяет её и обязан иметь что
// сверять.
func TestVersionIsSet(t *testing.T) {
	if Version == "" {
		t.Error("версия ядра пуста — реплею нечего сверять")
	}
}
